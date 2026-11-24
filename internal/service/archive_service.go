package service

import (
	"context"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// ArchiveService publishes a campaign sent by this platform to the workspace's
// campaign archive, so a sent campaign shows up under its own URL without anyone
// copying metrics by hand. Imported campaigns are archived by the import handler;
// this is the path for campaigns this platform sends.
//
// Dependencies are held as closures rather than the full repository interfaces so
// the entry-building logic can be tested without a database.
type ArchiveService struct {
	loadBroadcast func(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error)
	loadTemplate  func(ctx context.Context, workspaceID, templateID string, version int64) (*domain.Template, error)
	upsert        func(ctx context.Context, workspaceID string, entry *domain.ArchiveEntry) error
	now           func() time.Time
}

// NewArchiveService wires the service to the repositories the archive is written through.
func NewArchiveService(
	broadcastRepo domain.BroadcastRepository,
	templateRepo domain.TemplateRepository,
	archiveRepo domain.CampaignArchiveRepository,
) *ArchiveService {
	return &ArchiveService{
		loadBroadcast: func(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error) {
			return broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
		},
		loadTemplate: func(ctx context.Context, workspaceID, templateID string, version int64) (*domain.Template, error) {
			return templateRepo.GetTemplateByID(ctx, workspaceID, templateID, version)
		},
		upsert: archiveRepo.Upsert,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// ArchiveSentBroadcast records one sent campaign. The broadcast supplies the title
// and audience; the template supplies the subject line and is recorded at the exact
// version that was sent, so a later edit does not change the archived campaign.
//
// A broadcast with no name falls back to the template name, then the template id, so
// the title is never empty. Audiences are recorded as the sending platform names them;
// a send with no audience leaves it empty, which the archive reads as "not recorded".
func (s *ArchiveService) ArchiveSentBroadcast(ctx context.Context, workspaceID, broadcastID, templateID string, templateVersion int64) error {
	if s == nil || s.upsert == nil || broadcastID == "" || templateID == "" {
		return nil
	}

	var title, listID string
	if s.loadBroadcast != nil {
		if b, err := s.loadBroadcast(ctx, workspaceID, broadcastID); err == nil && b != nil {
			title = strings.TrimSpace(b.Name)
			listID = b.Audience.List
		}
	}

	var subject string
	if s.loadTemplate != nil {
		if tpl, err := s.loadTemplate(ctx, workspaceID, templateID, templateVersion); err == nil && tpl != nil {
			if tpl.Email != nil {
				subject = tpl.Email.Subject
			}
			if title == "" {
				title = strings.TrimSpace(tpl.Name)
			}
		}
	}
	if title == "" {
		title = templateID
	}

	sentAt := time.Now().UTC()
	if s.now != nil {
		sentAt = s.now()
	}

	return s.upsert(ctx, workspaceID, &domain.ArchiveEntry{
		Slug:            archiveSlugForSend(sentAt, title, templateID),
		TemplateID:      templateID,
		TemplateVersion: templateVersion,
		Title:           title,
		Subject:         subject,
		SentAt:          sentAt,
		ListID:          listID,
	})
}

// archiveSlugForSend builds a stable, human-readable URL slug: the date, then the
// title with everything but ASCII letters and digits folded to dashes. Two campaigns
// with the same title on the same day share a slug, which is intended — the archive
// keeps the latest.
func archiveSlugForSend(sentAt time.Time, title, templateID string) string {
	date := sentAt.UTC().Format("2006-01-02")
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 60 {
		slug = strings.Trim(slug[:60], "-")
	}
	if slug == "" {
		fallback := strings.TrimPrefix(templateID, "mc_")
		if len(fallback) > 6 {
			fallback = fallback[:6]
		}
		if fallback == "" {
			fallback = "campaign"
		}
		return date + "-" + fallback
	}
	return date + "-" + slug
}
