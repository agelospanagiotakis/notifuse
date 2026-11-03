package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// campaignArchiveRepository reads and writes the public campaign archive.
//
// The table is created on first use rather than through a versioned migration.
// That is a deliberate compromise, not an oversight: the migration registry is
// keyed to the upstream release number (v40) and selects work with
// `migrationVersion <= currentCodeVersion`, so a table introduced at 41 would
// never run on a v40 build, and moving the database version stamp to 41 would
// collide with upstream's numbering for every future release. The DDL below is
// additive and idempotent, so it is safe to attempt from any instance.
type campaignArchiveRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewCampaignArchiveRepository creates the archive repository
func NewCampaignArchiveRepository(workspaceRepo domain.WorkspaceRepository) domain.CampaignArchiveRepository {
	return &campaignArchiveRepository{workspaceRepo: workspaceRepo}
}

const archiveSchema = `
CREATE TABLE IF NOT EXISTS campaign_archive (
    slug             TEXT PRIMARY KEY,
    template_id      TEXT NOT NULL,
    template_version BIGINT NOT NULL DEFAULT 0,
    title            TEXT NOT NULL,
    subject          TEXT NOT NULL DEFAULT '',
    sent_at          TIMESTAMPTZ NOT NULL,
    archived_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS campaign_archive_sent_at_idx ON campaign_archive (sent_at DESC);
-- Reported figures, where a campaign was sent by another platform (Mailchimp). Kept
-- on the entry because the archive page shows them beside the campaign, and kept in
-- these columns rather than folded into Notifuse's own statistics because they were
-- measured elsewhere: whoever reads them later has to be able to tell.
ALTER TABLE campaign_archive ADD COLUMN IF NOT EXISTS sent_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_archive ADD COLUMN IF NOT EXISTS unique_opens INTEGER NOT NULL DEFAULT 0;
-- The audience a campaign went to. Empty means "not recorded" (every campaign imported
-- before audiences were captured), never "an audience with no name": grouping on '' would
-- invent a list nobody subscribed to.
ALTER TABLE campaign_archive ADD COLUMN IF NOT EXISTS list_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS campaign_archive_list_id_idx ON campaign_archive (list_id, sent_at DESC);
-- One row per audience, holding the URL slug and the display name. The slug is stored
-- rather than derived from the name because a list gets renamed and a URL must not move.
CREATE TABLE IF NOT EXISTS campaign_audience (
    list_id    TEXT PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Whether the audience is listed publicly. Default true so an audience registered by a
-- script (seed_audiences.sql) is visible unless someone deliberately unticks it; the
-- console is where the decision is made.
ALTER TABLE campaign_audience ADD COLUMN IF NOT EXISTS is_listed BOOLEAN NOT NULL DEFAULT TRUE;
`

func (r *campaignArchiveRepository) connection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	return db, nil
}

// EnsureSchema creates the archive table when it is missing.
func (r *campaignArchiveRepository) EnsureSchema(ctx context.Context, workspaceID string) error {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, archiveSchema); err != nil {
		return fmt.Errorf("failed to create campaign archive schema: %w", err)
	}
	return nil
}

// List returns every archived campaign, newest first.
func (r *campaignArchiveRepository) List(ctx context.Context, workspaceID string) ([]*domain.ArchiveEntry, error) {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT slug, template_id, template_version, title, subject, sent_at, archived_at,
		       sent_count, unique_opens, list_id
		FROM campaign_archive
		ORDER BY sent_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list campaign archive: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]*domain.ArchiveEntry, 0, 64)
	for rows.Next() {
		var e domain.ArchiveEntry
		if err := rows.Scan(&e.Slug, &e.TemplateID, &e.TemplateVersion, &e.Title, &e.Subject, &e.SentAt, &e.ArchivedAt, &e.SentCount, &e.UniqueOpens, &e.ListID); err != nil {
			return nil, fmt.Errorf("failed to scan campaign archive row: %w", err)
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read campaign archive: %w", err)
	}
	return entries, nil
}

// Get returns one archived campaign by slug.
func (r *campaignArchiveRepository) Get(ctx context.Context, workspaceID, slug string) (*domain.ArchiveEntry, error) {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	var e domain.ArchiveEntry
	err = db.QueryRowContext(ctx, `
		SELECT slug, template_id, template_version, title, subject, sent_at, archived_at,
		       sent_count, unique_opens, list_id
		FROM campaign_archive
		WHERE slug = $1`, slug).
		Scan(&e.Slug, &e.TemplateID, &e.TemplateVersion, &e.Title, &e.Subject, &e.SentAt, &e.ArchivedAt, &e.SentCount, &e.UniqueOpens, &e.ListID)
	if err == sql.ErrNoRows {
		return nil, domain.ErrArchiveEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign archive entry: %w", err)
	}
	return &e, nil
}

// Upsert inserts or refreshes an entry, so re-running an import is harmless.
func (r *campaignArchiveRepository) Upsert(ctx context.Context, workspaceID string, e *domain.ArchiveEntry) error {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return err
	}

	archivedAt := e.ArchivedAt
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}

	// list_id is updated with NULLIF so an upsert that does not know the audience leaves the
	// stored one alone: re-running an import must not erase what a previous run discovered.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_archive (slug, template_id, template_version, title, subject, sent_at, archived_at, list_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (slug) DO UPDATE SET
			template_id = EXCLUDED.template_id,
			template_version = EXCLUDED.template_version,
			title = EXCLUDED.title,
			subject = EXCLUDED.subject,
			sent_at = EXCLUDED.sent_at,
			list_id = NULLIF(EXCLUDED.list_id, '')`,
		e.Slug, e.TemplateID, e.TemplateVersion, e.Title, e.Subject, e.SentAt, archivedAt, e.ListID)
	if err != nil {
		return fmt.Errorf("failed to upsert campaign archive entry: %w", err)
	}
	return nil
}

// Remove deletes one archive row.
func (r *campaignArchiveRepository) Remove(ctx context.Context, workspaceID, slug string) error {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM campaign_archive WHERE slug = $1`, slug); err != nil {
		return err
	}
	return nil
}

// ListAudiences returns every audience that received at least one campaign, with its count.
//
// The join is a LEFT JOIN on purpose: a campaign whose audience has no registry row still gets
// counted, under a slug derived from its list id. Losing campaigns because nobody registered the
// audience would hide exactly the ones nobody remembered to set up.
func (r *campaignArchiveRepository) ListAudiences(ctx context.Context, workspaceID string) ([]*domain.ArchiveAudience, error) {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT a.list_id,
		       COALESCE(u.slug, ''),
		       COALESCE(u.name, ''),
		       a.campaigns,
		       COALESCE(u.is_listed, TRUE)
		FROM (SELECT list_id, COUNT(*) AS campaigns
		      FROM campaign_archive
		      WHERE list_id <> ''
		      GROUP BY list_id) a
		LEFT JOIN campaign_audience u ON u.list_id = a.list_id
		ORDER BY a.campaigns DESC, a.list_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list archive audiences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	audiences := make([]*domain.ArchiveAudience, 0, 8)
	for rows.Next() {
		var a domain.ArchiveAudience
		if err := rows.Scan(&a.ListID, &a.Slug, &a.Name, &a.Campaigns, &a.Listed); err != nil {
			return nil, fmt.Errorf("failed to scan archive audience row: %w", err)
		}
		audiences = append(audiences, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read archive audiences: %w", err)
	}
	return audiences, nil
}

// ListByListID returns the campaigns sent to one audience, newest first.
func (r *campaignArchiveRepository) ListByListID(ctx context.Context, workspaceID, listID string) ([]*domain.ArchiveEntry, error) {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT slug, template_id, template_version, title, subject, sent_at, archived_at,
		       sent_count, unique_opens, list_id
		FROM campaign_archive
		WHERE list_id = $1
		ORDER BY sent_at DESC`, listID)
	if err != nil {
		return nil, fmt.Errorf("failed to list campaign archive by audience: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]*domain.ArchiveEntry, 0, 64)
	for rows.Next() {
		var e domain.ArchiveEntry
		if err := rows.Scan(&e.Slug, &e.TemplateID, &e.TemplateVersion, &e.Title, &e.Subject, &e.SentAt, &e.ArchivedAt, &e.SentCount, &e.UniqueOpens, &e.ListID); err != nil {
			return nil, fmt.Errorf("failed to scan campaign archive row: %w", err)
		}
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read campaign archive: %w", err)
	}
	return entries, nil
}

// GetAudience resolves an audience by the slug in its URL.
func (r *campaignArchiveRepository) GetAudience(ctx context.Context, workspaceID, slug string) (*domain.ArchiveAudience, error) {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	var a domain.ArchiveAudience
	err = db.QueryRowContext(ctx, `
		SELECT u.list_id, u.slug, u.name, u.is_listed,
		       (SELECT COUNT(*) FROM campaign_archive c WHERE c.list_id = u.list_id)
		FROM campaign_audience u
		WHERE u.slug = $1 OR u.list_id = $1`, slug).
		Scan(&a.ListID, &a.Slug, &a.Name, &a.Listed, &a.Campaigns)
	if err == sql.ErrNoRows {
		return nil, domain.ErrArchiveAudienceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get archive audience: %w", err)
	}
	return &a, nil
}

// UpsertAudience records an audience's slug and display name, so re-running the import is safe.
func (r *campaignArchiveRepository) UpsertAudience(ctx context.Context, workspaceID string, a *domain.ArchiveAudience) error {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	// is_listed is written as true on insert and left alone on update: re-running an import
	// must not silently re-list an audience the operator unticked.
	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_audience (list_id, slug, name, is_listed)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (list_id) DO UPDATE SET slug = EXCLUDED.slug, name = EXCLUDED.name`,
		a.ListID, a.Slug, a.Name)
	if err != nil {
		return fmt.Errorf("failed to upsert archive audience: %w", err)
	}
	return nil
}

// SetAudienceListed ticks or unticks one audience in the public archive.
func (r *campaignArchiveRepository) SetAudienceListed(ctx context.Context, workspaceID, listID string, listed bool) error {
	db, err := r.connection(ctx, workspaceID)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx,
		`UPDATE campaign_audience SET is_listed = $2 WHERE list_id = $1`, listID, listed)
	if err != nil {
		return fmt.Errorf("failed to set audience listing: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrArchiveAudienceNotFound
	}
	return nil
}
