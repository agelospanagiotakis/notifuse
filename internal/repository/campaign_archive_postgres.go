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
		SELECT slug, template_id, template_version, title, subject, sent_at, archived_at
		FROM campaign_archive
		ORDER BY sent_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list campaign archive: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]*domain.ArchiveEntry, 0, 64)
	for rows.Next() {
		var e domain.ArchiveEntry
		if err := rows.Scan(&e.Slug, &e.TemplateID, &e.TemplateVersion, &e.Title, &e.Subject, &e.SentAt, &e.ArchivedAt); err != nil {
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
		SELECT slug, template_id, template_version, title, subject, sent_at, archived_at
		FROM campaign_archive
		WHERE slug = $1`, slug).
		Scan(&e.Slug, &e.TemplateID, &e.TemplateVersion, &e.Title, &e.Subject, &e.SentAt, &e.ArchivedAt)
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

	_, err = db.ExecContext(ctx, `
		INSERT INTO campaign_archive (slug, template_id, template_version, title, subject, sent_at, archived_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (slug) DO UPDATE SET
			template_id = EXCLUDED.template_id,
			template_version = EXCLUDED.template_version,
			title = EXCLUDED.title,
			subject = EXCLUDED.subject,
			sent_at = EXCLUDED.sent_at`,
		e.Slug, e.TemplateID, e.TemplateVersion, e.Title, e.Subject, e.SentAt, archivedAt)
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
