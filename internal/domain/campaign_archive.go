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
	// ListID is the audience the campaign was sent to, as the sending platform named it.
	// Empty means "not recorded" — every campaign imported before audiences were captured,
	// and every campaign this platform sent without one. An empty value is never treated as
	// an audience of its own.
	ListID string `json:"list_id,omitempty"`
	// SentCount and UniqueOpens are reported figures for a campaign sent elsewhere.
	// Zero means "not known", never "nobody opened": an imported campaign that has no
	// report must not be displayed as a failure.
	SentCount   int `json:"sent_count"`
	UniqueOpens int `json:"unique_opens"`
}

// ArchiveAudience is one audience's home in the archive.
//
// Mailchimp's campaign archive is per audience: each list has its own page and its own feed.
// Ours was one page for the workspace, which answers "what was sent" but not "what was sent to
// THIS list" — a question the client's history makes real, because the same campaign was rarely
// sent to every audience (105 of 182 went to one list, 69 to a different one).
//
// The slug is stored rather than derived so a URL can be chosen once and stay stable: the
// display name is whatever the sending platform called the list, and that name changes.
type ArchiveAudience struct {
	ListID string `json:"list_id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	// Listed is whether this audience is offered publicly: in the archive's audience switch
	// and at its own URL. Unticked, the page answers 404 — "not listed" is a visibility
	// decision, and a page that still resolves is not hidden.
	Listed    bool `json:"listed"`
	Campaigns int  `json:"campaigns"`
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
	// ShowAudienceNav puts the audience switch on the PUBLIC archive pages. Off by
	// default, because which list a campaign went to is running information, not publishing
	// information: a reader on an offer page has no use for "this went to 11,924 people on
	// Kois Optics". The per-audience pages still exist and stay linkable — the console lists
	// them for the people who need them.
	ShowAudienceNav bool `json:"show_audience_nav,omitempty"`
}

// ErrArchiveEntryNotFound is returned when a slug is not in the archive.
var ErrArchiveEntryNotFound = fmt.Errorf("campaign archive entry not found")

// CampaignArchiveRepository stores the public campaign archive per workspace.
type CampaignArchiveRepository interface {
	EnsureSchema(ctx context.Context, workspaceID string) error
	List(ctx context.Context, workspaceID string) ([]*ArchiveEntry, error)
	Get(ctx context.Context, workspaceID, slug string) (*ArchiveEntry, error)
	Upsert(ctx context.Context, workspaceID string, entry *ArchiveEntry) error
	Remove(ctx context.Context, workspaceID, slug string) error

	// ListAudiences returns every audience that actually received a campaign, with its
	// count. Audiences with no campaigns are not listed: an archive page for a list
	// nothing was ever sent to is a page nobody should land on.
	ListAudiences(ctx context.Context, workspaceID string) ([]*ArchiveAudience, error)
	// ListByListID returns the campaigns sent to one audience, newest first.
	ListByListID(ctx context.Context, workspaceID, listID string) ([]*ArchiveEntry, error)
	// GetAudience resolves an audience by its URL slug.
	GetAudience(ctx context.Context, workspaceID, slug string) (*ArchiveAudience, error)
	// SetAudienceListed ticks or unticks an audience in the public archive.
	SetAudienceListed(ctx context.Context, workspaceID, listID string, listed bool) error
	// UpsertAudience records an audience's slug and display name.
	UpsertAudience(ctx context.Context, workspaceID string, audience *ArchiveAudience) error
}

// ErrArchiveAudienceNotFound is returned when a slug is not a registered audience.
var ErrArchiveAudienceNotFound = fmt.Errorf("campaign archive audience not found")
