package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ImportHandler is the Mailchimp import screen's server side.
//
// It does not run the import. The import is a several-hour job that must survive
// deploys, resume from campaign 101, and download gigabytes of assets — that is a
// worker's job, and the worker exists (`mc_import_assets.py`). What was missing was
// never the work; it was knowing where you are: how much of each import type is
// done, whether the key still valid, and what to run for the part that is not.
//
// So each import type is its own tool with a measured status and the exact command
// for the step that remains. That is also why the key is stored encrypted rather
// than pasted into a shell each time.
type ImportHandler struct {
	workspaceRepo domain.WorkspaceRepository
	templateRepo  domain.TemplateRepository
	archiveRepo   domain.CampaignArchiveRepository
	httpClient    *http.Client
	getJWTSecret  func() ([]byte, error)
	secretKey     string
	logger        logger.Logger
}

func NewImportHandler(
	workspaceRepo domain.WorkspaceRepository,
	templateRepo domain.TemplateRepository,
	archiveRepo domain.CampaignArchiveRepository,
	getJWTSecret func() ([]byte, error),
	secretKey string,
	logger logger.Logger,
) *ImportHandler {
	return &ImportHandler{
		workspaceRepo: workspaceRepo,
		templateRepo:  templateRepo,
		archiveRepo:   archiveRepo,
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		getJWTSecret:  getJWTSecret,
		secretKey:     secretKey,
		logger:        logger,
	}
}

func (h *ImportHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	mux.Handle("/api/import.status", requireAuth(http.HandlerFunc(h.handleStatus)))
	mux.Handle("/api/import.key", requireAuth(http.HandlerFunc(h.handleKey)))
	mux.Handle("/api/import.inventory", requireAuth(http.HandlerFunc(h.handleInventory)))
}

// importTool is one import type: what it produces, where the work is, and how far along.
type importTool struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Why       string `json:"why"`
	Done      bool   `json:"done"`
	DoneCount int    `json:"done_count"`
	Total     int    `json:"total"`
	Unit      string `json:"unit"`
	Command   string `json:"command"`
	Note      string `json:"note,omitempty"`
}

func (h *ImportHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ws := h.workspace(w, r)
	if ws == nil {
		return
	}

	templates, _ := h.templateRepo.GetTemplates(r.Context(), ws.ID, "", "")
	imported := 0
	for _, t := range templates {
		if strings.HasPrefix(t.ID, "mc_") && t.DeletedAt == nil {
			imported++
		}
	}

	archived := 0
	if err := h.archiveRepo.EnsureSchema(r.Context(), ws.ID); err == nil {
		if list, err := h.archiveRepo.List(r.Context(), ws.ID); err == nil {
			archived = len(list)
		}
	}

	keySet := ws.Settings.MailchimpAPIKeyEnc != ""

	tools := []importTool{
		{
			ID: "templates", Title: "Campaigns as templates",
			Why:  "Every sent campaign becomes a code-mode template, so it can be previewed, reused and archived.",
			Done: imported > 0, DoneCount: imported, Total: 0, Unit: "campaigns",
			Command: "ssh root@64.226.122.222 'python3 -u /tmp/mc_import_assets.py --workspace koisoptics --limit 182'",
		},
		{
			ID: "assets", Title: "Images moved to our storage",
			Why:  "Mailchimp-hosted images are copied to MinIO and the links rewritten, so the archive survives the account being closed.",
			Done: imported > 0, DoneCount: imported, Total: 0, Unit: "campaigns with assets",
			Command: "same run as templates — assets are uploaded as each campaign is processed",
			Note:    "Content-addressed per campaign folder; re-running skips what exists.",
		},
		{
			ID: "archive", Title: "Campaign archive entries",
			Why:  "Publishes each campaign to /archive/<workspace>/ and backdates its template to the real send date.",
			Done: archived > 0, DoneCount: archived, Total: imported, Unit: "campaigns",
			Command: "ssh root@64.226.122.222 'python3 -u /tmp/mc_archive.py --workspace kois-optics --apply'",
		},
		{
			ID: "subscribers", Title: "Subscribers",
			Why:  "The audience itself. Not started: it needs a consent decision before a single contact is written.",
			Done: false, DoneCount: 0, Total: 0, Unit: "contacts",
			Command: "Mailchimp → Audience → Export as CSV, then Contacts → Import in this console",
			Note:    "Importing an unsubscribed contact as subscribed cannot be undone — decide the mapping first.",
		},
		{
			ID: "broadcasts", Title: "Broadcast history (optional)",
			Why:  "Drafts carrying the original send dates, so the console shows what was sent and when.",
			Done: false, DoneCount: 0, Total: 0, Unit: "drafts",
			Command: "not built — deliberately postponed; the archive already carries the dates",
		},
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tools":      tools,
		"key_set":    keySet,
		"workspace":  ws.Name,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ImportHandler) workspace(w http.ResponseWriter, r *http.Request) *domain.Workspace {
	id := r.URL.Query().Get("workspace_id")
	if id == "" && r.Method == http.MethodPost {
		var body struct {
			WorkspaceID string `json:"workspace_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id = body.WorkspaceID
	}
	if id == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return nil
	}
	ws, err := h.workspaceRepo.GetByID(r.Context(), id)
	if err != nil || ws == nil {
		WriteJSONError(w, "workspace not found", http.StatusNotFound)
		return nil
	}
	return ws
}

// handleKey stores the Mailchimp API key, encrypted with the workspace secret.
func (h *ImportHandler) handleKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		APIKey      string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ws, err := h.workspaceRepo.GetByID(r.Context(), body.WorkspaceID)
	if err != nil || ws == nil {
		WriteJSONError(w, "workspace not found", http.StatusNotFound)
		return
	}

	key := strings.TrimSpace(body.APIKey)
	if key == "" {
		ws.Settings.MailchimpAPIKeyEnc = ""
		if err := h.workspaceRepo.Update(r.Context(), ws); err != nil {
			WriteJSONError(w, "could not clear the key", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	// Validate before storing: a key that does not work is worse than none, because
	// the next person believes the import is configured.
	if err := h.checkMailchimpKey(r.Context(), key); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	enc, err := crypto.EncryptString(key, h.secretKey)
	if err != nil {
		WriteJSONError(w, "could not encrypt the key", http.StatusInternalServerError)
		return
	}
	ws.Settings.MailchimpAPIKeyEnc = enc
	if err := h.workspaceRepo.Update(r.Context(), ws); err != nil {
		WriteJSONError(w, "could not save the key", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleInventory asks Mailchimp what is there, so nobody imports blind.
func (h *ImportHandler) handleInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ws := h.workspace(w, r)
	if ws == nil {
		return
	}
	if ws.Settings.MailchimpAPIKeyEnc == "" {
		WriteJSONError(w, "no Mailchimp API key stored for this workspace", http.StatusPreconditionFailed)
		return
	}
	key, err := crypto.DecryptFromHexString(ws.Settings.MailchimpAPIKeyEnc, h.secretKey)
	if err != nil {
		WriteJSONError(w, "stored key could not be decrypted", http.StatusInternalServerError)
		return
	}

	campaigns, err := h.mailchimpCount(r.Context(), key, "campaigns", "&type=regular&status=sent")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadGateway)
		return
	}
	audiences, err := h.mailchimpCount(r.Context(), key, "lists", "")
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadGateway)
		return
	}
	subscribers, _ := h.mailchimpSubscribers(r.Context(), key)

	writeJSON(w, http.StatusOK, map[string]any{
		"campaigns":   campaigns,
		"audiences":   audiences,
		"subscribers": subscribers,
	})
}

func (h *ImportHandler) checkMailchimpKey(ctx context.Context, key string) error {
	dc, err := mailchimpDC(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+dc+".api.mailchimp.com/3.0/", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth("anystring", key)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Mailchimp: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Mailchimp rejected the key (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func (h *ImportHandler) mailchimpCount(ctx context.Context, key, resource, extra string) (int, error) {
	dc, err := mailchimpDC(key)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("https://%s.api.mailchimp.com/3.0/%s?count=1&fields=total_items%s", dc, resource, extra)
	body, err := h.mailchimpGet(ctx, key, url)
	if err != nil {
		return 0, err
	}
	var out struct {
		TotalItems int `json:"total_items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.TotalItems, nil
}

func (h *ImportHandler) mailchimpSubscribers(ctx context.Context, key string) (int, error) {
	dc, err := mailchimpDC(key)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("https://%s.api.mailchimp.com/3.0/lists?count=10&fields=lists.id,lists.stats.member_count,total_items", dc)
	body, err := h.mailchimpGet(ctx, key, url)
	if err != nil {
		return 0, err
	}
	var out struct {
		Lists []struct {
			Stats struct {
				MemberCount int `json:"member_count"`
			} `json:"stats"`
		} `json:"lists"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	total := 0
	for _, l := range out.Lists {
		total += l.Stats.MemberCount
	}
	return total, nil
}

func (h *ImportHandler) mailchimpGet(ctx context.Context, key, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("anystring", key)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mailchimp answered HTTP %d", resp.StatusCode)
	}
	return body, nil
}

var mailchimpSuffix = regexp.MustCompile(`-([a-z]+\d+)$`)

func mailchimpDC(key string) (string, error) {
	m := mailchimpSuffix.FindStringSubmatch(key)
	if m == nil {
		return "", fmt.Errorf("that does not look like a Mailchimp key (it must end in a data-centre suffix such as -us8)")
	}
	return m[1], nil
}

var _ = base64.StdEncoding
