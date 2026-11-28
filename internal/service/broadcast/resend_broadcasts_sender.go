package broadcast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/google/uuid"
)

const resendAPIBaseURL = "https://api.resend.com"

// resendBroadcastsSender is a deliberately small beta sender for Resend Marketing
// Broadcasts. It trades Notifuse's per-recipient queue and tracking for Resend's
// Broadcasts/Audiences quota:
//
//   - one Resend audience and one Broadcast are created per orchestrator batch;
//   - contacts are uploaded by email only (no per-contact Liquid rendering);
//   - no message_history rows, open/click tracking, A/B tests or test phases;
//   - unsubscribe and delivery are handled by Resend, not by Notifuse.
//
// The normal Resend SMTP provider remains the default and is unaffected.
type resendBroadcastsSender struct {
	client        *http.Client
	apiBase       string
	broadcastRepo domain.BroadcastRepository
	logger        logger.Logger
}

func NewResendBroadcastsSender(broadcastRepo domain.BroadcastRepository, l logger.Logger) MessageSender {
	return &resendBroadcastsSender{
		client:        &http.Client{Timeout: 2 * time.Minute},
		apiBase:       resendAPIBaseURL,
		broadcastRepo: broadcastRepo,
		logger:        l,
	}
}

func (s *resendBroadcastsSender) SendToRecipient(
	_ context.Context, _, _, _, _ string,
	_ domain.EmailTrackingDecision,
	_ *domain.WebAnalyticsSettings,
	_ *domain.Broadcast,
	_ string,
	_ string,
	_ *domain.Template,
	_ map[string]interface{},
	_ *domain.EmailProvider,
	_ time.Time,
	_, _ string,
) error {
	return fmt.Errorf("resend broadcasts beta only supports batch broadcast sends")
}

func (s *resendBroadcastsSender) SendBatch(
	ctx context.Context,
	workspaceID string,
	_ string,
	_ string,
	_ string,
	_ string,
	_ domain.EmailTrackingPolicy,
	_ *domain.WebAnalyticsSettings,
	broadcastID string,
	recipients []*domain.ContactWithList,
	templates map[string]*domain.Template,
	emailProvider *domain.EmailProvider,
	_ time.Time,
	workspaceDefaultLanguage string,
) (int, int, error) {
	if emailProvider == nil || emailProvider.ResendBroadcasts == nil || emailProvider.ResendBroadcasts.APIKey == "" {
		return 0, 0, fmt.Errorf("resend broadcasts API key is not configured")
	}
	if len(recipients) == 0 {
		return 0, 0, nil
	}

	broadcast, err := s.loadBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return 0, len(recipients), err
	}
	if broadcast.TestSettings.Enabled {
		return 0, 0, fmt.Errorf("resend broadcasts beta does not support A/B tests or test phases")
	}
	if len(templates) == 0 {
		return 0, len(recipients), fmt.Errorf("no template selected for broadcast")
	}

	template := firstTemplate(templates)
	content := template.EmailContentForLanguage(workspaceDefaultLanguage)
	if content == nil {
		return 0, len(recipients), fmt.Errorf("email content not available")
	}
	sender, err := emailProvider.ResolveSender(broadcast.VariationSenderID(template.ID), content.SenderID)
	if err != nil {
		return 0, len(recipients), err
	}

	// No per-contact data: this beta intentionally renders one generic message for
	// the whole batch. Per-recipient Liquid, tracking links and A/B are not supported.
	data := map[string]interface{}{}
	messageID := fmt.Sprintf("reb_%s_%s", broadcastID, uuid.New().String())
	compileReq := notifuse_mjml.CompileTemplateRequest{
		WorkspaceID:  workspaceID,
		MessageID:    messageID,
		TemplateData: data,
	}
	content.ApplyToCompileRequest(&compileReq, nil)
	compiled, err := notifuse_mjml.CompileTemplate(compileReq)
	if err != nil {
		return 0, len(recipients), fmt.Errorf("failed to compile template for Resend Broadcasts: %w", err)
	}
	if !compiled.Success || compiled.HTML == nil {
		msg := "template compilation failed"
		if compiled.Error != nil {
			msg = compiled.Error.Message
		}
		return 0, len(recipients), fmt.Errorf("%s", msg)
	}
	subject, err := notifuse_mjml.ProcessLiquidTemplate(content.Subject, data, "email_subject")
	if err != nil {
		subject = content.Subject
	}

	audienceID, err := s.createAudience(ctx, emailProvider.ResendBroadcasts.APIKey, broadcastID)
	if err != nil {
		return 0, len(recipients), err
	}
	if err := s.addAudienceContacts(ctx, emailProvider.ResendBroadcasts.APIKey, audienceID, recipients); err != nil {
		return 0, len(recipients), err
	}

	from := fmt.Sprintf("%s <%s>", sender.Name, sender.Email)
	resendBroadcastID, err := s.createBroadcast(ctx, emailProvider.ResendBroadcasts.APIKey, audienceID, from, subject, *compiled.HTML, broadcast.Name, content.ReplyTo)
	if err != nil {
		return 0, len(recipients), err
	}
	if err := s.sendBroadcast(ctx, emailProvider.ResendBroadcasts.APIKey, resendBroadcastID); err != nil {
		return 0, len(recipients), err
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id":           workspaceID,
		"broadcast_id":           broadcastID,
		"resend_audience_id":     audienceID,
		"resend_broadcast_id":    resendBroadcastID,
		"recipients":             len(recipients),
		"provider":               "resend_broadcasts",
		"per_recipient_tracking": false,
	}).Info("Resend Broadcast sent for batch")

	return len(recipients), 0, nil
}

func firstTemplate(templates map[string]*domain.Template) *domain.Template {
	ids := make([]string, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return templates[ids[0]]
}

func (s *resendBroadcastsSender) loadBroadcast(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error) {
	broadcast, err := s.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return nil, fmt.Errorf("failed to load broadcast: %w", err)
	}
	if broadcast == nil {
		return nil, fmt.Errorf("broadcast %s not found", broadcastID)
	}
	return broadcast, nil
}

func (s *resendBroadcastsSender) createAudience(ctx context.Context, apiKey, broadcastID string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	name := fmt.Sprintf("notifuse-%s-%s", trimID(broadcastID), uuid.New().String()[:8])
	if err := s.post(ctx, apiKey, "/audiences", map[string]interface{}{"name": name}, &out); err != nil {
		return "", fmt.Errorf("create Resend audience: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("create Resend audience returned no id")
	}
	return out.ID, nil
}

func (s *resendBroadcastsSender) addAudienceContacts(ctx context.Context, apiKey, audienceID string, recipients []*domain.ContactWithList) error {
	seen := make(map[string]struct{}, len(recipients))
	emails := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if r == nil || r.Contact == nil || strings.TrimSpace(r.Contact.Email) == "" {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(r.Contact.Email))
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	var mu sync.Mutex
	var firstErr error
	for _, email := range emails {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			err := s.post(ctx, apiKey, "/audiences/"+audienceID+"/contacts", map[string]interface{}{
				"email":        email,
				"unsubscribed": false,
			}, nil)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(email)
	}
	wg.Wait()
	if firstErr != nil {
		return fmt.Errorf("add Resend audience contacts: %w", firstErr)
	}
	return nil
}

func (s *resendBroadcastsSender) createBroadcast(ctx context.Context, apiKey, audienceID, from, subject, html, name, replyTo string) (string, error) {
	body := map[string]interface{}{
		"audience_id": audienceID,
		"from":        from,
		"subject":     subject,
		"html":        html,
		"name":        name,
	}
	if replyTo != "" {
		body["reply_to"] = replyTo
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := s.post(ctx, apiKey, "/broadcasts", body, &out); err != nil {
		return "", fmt.Errorf("create Resend broadcast: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("create Resend broadcast returned no id")
	}
	return out.ID, nil
}

func (s *resendBroadcastsSender) sendBroadcast(ctx context.Context, apiKey, broadcastID string) error {
	if err := s.post(ctx, apiKey, "/broadcasts/"+broadcastID+"/send", map[string]interface{}{}, nil); err != nil {
		return fmt.Errorf("send Resend broadcast: %w", err)
	}
	return nil
}

func (s *resendBroadcastsSender) post(ctx context.Context, apiKey, path string, body interface{}, out interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBase+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Resend API %s returned HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode Resend API response: %w", err)
		}
	}
	return nil
}

func trimID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
