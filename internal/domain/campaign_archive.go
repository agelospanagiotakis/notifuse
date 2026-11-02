package domain

import (
	"context"
	"fmt"
	"time"
)

// ArchiveEntry is one campaign in a workspace's public campaign archive.
//
// This is the piece Notifuse has no equivalent of. A broadcast records that mail
// was sent; a blog post publishes something to a themed site. Mailchimp's
// campaign archive is a third thing: the sent campaign itself, addressable by a
// URL, permanent, and public — the page a customer lands on when they Google a
// newsletter they were sent a year ago.
//
// The entry is a pointer, not a copy: it names the template and version that was
// sent, so the archive renders the campaign from the same source the email was
// built from. Copying the HTML would let the two drift the first time someone
// edited the template.
type ArchiveEntry struct {
	Slug            string    `json:"slug"`
	TemplateID      string    `json:"template_id"`
	TemplateVersion int64     `json:"template_version"`
	Title           string    `json:"title"`
	Subject         string    `json:"subject,omitempty"`
	SentAt          time.Time `json:"sent_at"`
	ArchivedAt      time.Time `json:"archived_at"`
}

// ArchiveSettings is stored in the workspace settings blob under "archive".
//
// BasicAuthUser/Password are a temporary gate: the archive is meant to be public
// in the end, but nothing should be indexable before the content is checked. An
// empty user means "no gate", which is also how the feature is turned public
// later — no code change, just clear the field.
type ArchiveSettings struct {
	Enabled           bool   `json:"enabled"`
	Title             string `json:"title,omitempty"`
	BasicAuthUser     string `json:"basic_auth_user,omitempty"`
	BasicAuthPassword string `json:"basic_auth_password,omitempty"`
	// SubscribeURL is where the toolbar's "Subscribe" points. Mailchimp's archive
	// offers signup in the page itself because the archive and the audience are one
	// product; here the signup form lives in the workspace's own site, so this is a
	// setting rather than something the page can invent.
	SubscribeURL string `json:"subscribe_url,omitempty"`
	// SubscribeListIDs are the public lists the archive's own signup form joins.
	// When set, the toolbar's Subscribe goes to a form we render and serve; the
	// external SubscribeURL is the fallback for a workspace that would rather keep
	// signup on its own site.
	SubscribeListIDs []string `json:"subscribe_list_ids,omitempty"`
}

// ErrArchiveEntryNotFound is returned when a slug is not in the archive.
var ErrArchiveEntryNotFound = fmt.Errorf("campaign archive entry not found")

// CampaignArchiveRepository stores the public campaign archive per workspace.
type CampaignArchiveRepository interface {
	EnsureSchema(ctx context.Context, workspaceID string) error
	List(ctx context.Context, workspaceID string) ([]*ArchiveEntry, error)
	Get(ctx context.Context, workspaceID, slug string) (*ArchiveEntry, error)
	Upsert(ctx context.Context, workspaceID string, entry *ArchiveEntry) error
}
