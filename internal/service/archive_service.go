package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// ArchiveService records campaigns this platform sends in the workspace's public archive.
//
// The archive used to be written only by the Mailchimp import and by an operator publishing a
// template, which means it answered "what did we migrate" and not "what have we sent". That is
// backwards: the archive is the promise that a campaign stays addressable forever, and a
// campaign sent from here is exactly the one a customer will look for next year.
//
// Everything here is idempotent. The hook that calls it runs at the end of a task that can be
// retried, and a retried send must not publish the same campaign twice.
type ArchiveService struct {
	broadcastRepo domain.BroadcastRepository
	templateRepo  domain.TemplateRepository
	listRepo      domain.ListRepository
	archiveRepo   domain.CampaignArchiveRepository
	logger        logger.Logger
}

func NewArchiveService(
	broadcastRepo domain.BroadcastRepository,
	templateRepo domain.TemplateRepository,
	listRepo domain.ListRepository,
	archiveRepo domain.CampaignArchiveRepository,
	logger logger.Logger,
) *ArchiveService {
	return &ArchiveService{
		broadcastRepo: broadcastRepo,
		templateRepo:  templateRepo,
		listRepo:      listRepo,
		archiveRepo:   archiveRepo,
		logger:        logger,
	}
}

// ArchiveSentBroadcast writes the archive entry for a broadcast the platform has just sent.
//
// templateID/templateVersion are what the send actually used — passed in rather than read back,
// because a template edited during a long send would otherwise be archived at a version that
// was never delivered.
func (s *ArchiveService) ArchiveSentBroadcast(ctx context.Context, workspaceID, broadcastID, templateID string, templateVersion int64) error {
	broadcast, err := s.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		return fmt.Errorf("failed to load broadcast: %w", err)
	}

	// Only a completed send is archived. A cancelled or paused broadcast may have mailed some
	// recipients, but publishing it would tell the public it went out.
	if broadcast.Status != domain.BroadcastStatusProcessed || broadcast.CancelledAt != nil {
		return nil
	}

	listID := strings.TrimSpace(broadcast.Audience.List)
	if listID == "" {
		// No audience means no page it could belong to. Not an error: the archive's
		// per-audience homes are the only way a campaign is reached.
		return nil
	}

	if templateID == "" {
		templateID = sentTemplateID(broadcast)
	}
	if templateID == "" {
		return fmt.Errorf("broadcast %s has no template to archive", broadcastID)
	}
	if templateVersion == 0 {
		tpl, err := s.templateFor(ctx, workspaceID, templateID)
		if err != nil {
			return err
		}
		templateVersion = tpl.Version
	}

	sentAt := time.Now().UTC()
	switch {
	case broadcast.StartedAt != nil:
		sentAt = broadcast.StartedAt.UTC()
	case broadcast.CompletedAt != nil:
		sentAt = broadcast.CompletedAt.UTC()
	}

	title := strings.TrimSpace(broadcast.Name)
	if title == "" {
		title = "Campaign"
	}

	if err := s.archiveRepo.EnsureSchema(ctx, workspaceID); err != nil {
		return fmt.Errorf("failed to ensure archive schema: %w", err)
	}

	// A retried task finds its own entry and updates it. Without this the second run would
	// publish the same campaign under a second slug.
	existing, err := s.archiveRepo.GetByBroadcastID(ctx, workspaceID, broadcastID)
	slug := ""
	if err == nil && existing != nil {
		slug = existing.Slug
	} else if err != domain.ErrArchiveEntryNotFound && err != nil {
		return fmt.Errorf("failed to look up existing archive entry: %w", err)
	}
	if slug == "" {
		slug, err = s.uniqueSlug(ctx, workspaceID, sentAt, title, broadcastID)
		if err != nil {
			return err
		}
	}

	subject := ""
	if tpl, tplErr := s.templateFor(ctx, workspaceID, templateID); tplErr == nil && tpl.Email != nil {
		subject = tpl.Email.Subject
	}

	entry := &domain.ArchiveEntry{
		Slug:            slug,
		TemplateID:      templateID,
		TemplateVersion: templateVersion,
		Title:           title,
		Subject:         subject,
		SentAt:          sentAt,
		ListID:          listID,
		BroadcastID:     broadcastID,
	}
	if err := s.archiveRepo.Upsert(ctx, workspaceID, entry); err != nil {
		return fmt.Errorf("failed to write archive entry: %w", err)
	}

	// The audience has to answer to the platform's own list id before the page will resolve.
	if err := s.ensureAudience(ctx, workspaceID, listID); err != nil {
		// The entry is already written and its URL works; the audience switch is the only thing
		// missing, so this is logged rather than turned into a failed send.
		s.logger.WithField("workspace_id", workspaceID).WithField("list_id", listID).
			WithField("error", err.Error()).Warn("Archived campaign but could not register its audience")
	}

	s.logger.WithField("workspace_id", workspaceID).
		WithField("broadcast_id", broadcastID).
		WithField("slug", slug).
		WithField("list_id", listID).
		Info("Archived sent campaign")

	return nil
}

// sentTemplateID picks the template the send used: the A/B winner when there was one, otherwise
// the first variation, which is where a single-template broadcast keeps its template.
func sentTemplateID(b *domain.Broadcast) string {
	if b.WinningTemplate != nil && *b.WinningTemplate != "" {
		return *b.WinningTemplate
	}
	if len(b.TestSettings.Variations) > 0 {
		return b.TestSettings.Variations[0].TemplateID
	}
	return ""
}

// templateFor reads a template's latest version (version 0 asks the repository for it).
func (s *ArchiveService) templateFor(ctx context.Context, workspaceID, templateID string) (*domain.Template, error) {
	tpl, err := s.templateRepo.GetTemplateByID(ctx, workspaceID, templateID, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to load template %s: %w", templateID, err)
	}
	return tpl, nil
}

// ensureAudience registers the platform list as an archive audience the first time one of its
// campaigns is archived, so the campaign lands on a page that exists.
func (s *ArchiveService) ensureAudience(ctx context.Context, workspaceID, listID string) error {
	if _, err := s.archiveRepo.GetAudience(ctx, workspaceID, listID); err == nil {
		return nil
	} else if err != domain.ErrArchiveAudienceNotFound {
		return err
	}

	name := listID
	if list, err := s.listRepo.GetListByID(ctx, workspaceID, listID); err == nil && list != nil && list.Name != "" {
		name = list.Name
	}

	slug := archiveTitleSlug(name)
	if slug == "" {
		slug = archiveTitleSlug(listID)
	}
	// A slug is a public URL and must be unique across audiences.
	base := slug
	for i := 2; i < 50; i++ {
		if _, err := s.archiveRepo.GetAudience(ctx, workspaceID, slug); err == domain.ErrArchiveAudienceNotFound {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}

	return s.archiveRepo.UpsertAudience(ctx, workspaceID, &domain.ArchiveAudience{
		ListID:         listID,
		Slug:           slug,
		Name:           name,
		NotifuseListID: listID,
	})
}

// uniqueSlug builds the campaign's URL: the send date, then the title, then the broadcast id when
// the readable part is already taken (two campaigns called "Προσφορά" in one day is normal).
func (s *ArchiveService) uniqueSlug(ctx context.Context, workspaceID string, sentAt time.Time, title, broadcastID string) (string, error) {
	base := sentAt.Format("2006-01-02") + "-" + archiveTitleSlug(title)
	if strings.HasSuffix(base, "-") || base == "" {
		base = sentAt.Format("2006-01-02") + "-campaign"
	}
	candidate := base
	taken, err := s.archiveRepo.SlugExists(ctx, workspaceID, candidate)
	if err != nil {
		return "", err
	}
	if !taken {
		return candidate, nil
	}
	suffix := broadcastID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return base + "-" + suffix, nil
}

// greekToLatin transliterates Greek letters to Latin ones.
//
// Without it a Greek title reduces to nothing and every campaign is called "campaign", so the
// archive's URLs are unreadable and collide on the suffix. The import's own slugs show the cost
// of skipping it: the September 2026 issue is "2026-09-14-200", because everything but the digits
// was dropped. Slugs written before this change keep whatever they were given — a URL that has
// been shared must not move.
var greekToLatin = map[rune]string{
	'α': "a", 'ά': "a", 'β': "v", 'γ': "g", 'δ': "d", 'ε': "e", 'έ': "e",
	'ζ': "z", 'η': "i", 'ή': "i", 'θ': "th", 'ι': "i", 'ί': "i", 'ϊ': "i", 'ΐ': "i",
	'κ': "k", 'λ': "l", 'μ': "m", 'ν': "n", 'ξ': "x", 'ο': "o", 'ό': "o",
	'π': "p", 'ρ': "r", 'σ': "s", 'ς': "s", 'τ': "t", 'υ': "y", 'ύ': "y", 'ϋ': "y", 'ΰ': "y",
	'φ': "f", 'χ': "ch", 'ψ': "ps", 'ω': "o", 'ώ': "o",
}

// archiveTitleSlug keeps letters and digits, transliterates Greek, and turns runs of anything
// else into one dash. It is the same shape the import produced for Latin titles
// ("2026-06-11-chimera-mona-lorenz"), so a new URL looks like its neighbours.
func archiveTitleSlug(s string) string {
	var b strings.Builder
	dash := false
	writeDash := func() {
		if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	for _, r := range strings.ToLower(s) {
		if mapped, ok := greekToLatin[r]; ok {
			b.WriteString(mapped)
			dash = false
			continue
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == '/' || r == ',' || r == ':':
			writeDash()
		}
	}
	return strings.Trim(b.String(), "-")
}
