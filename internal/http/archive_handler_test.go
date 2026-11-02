package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeArchiveRepo is a small in-memory stand-in: the archive repository is three
// queries, and a generated mock would not make these tests clearer.
type fakeArchiveRepo struct {
	entries []*domain.ArchiveEntry
	schema  int
}

func (f *fakeArchiveRepo) EnsureSchema(context.Context, string) error { f.schema++; return nil }
func (f *fakeArchiveRepo) Upsert(context.Context, string, *domain.ArchiveEntry) error {
	return nil
}
func (f *fakeArchiveRepo) List(context.Context, string) ([]*domain.ArchiveEntry, error) {
	return f.entries, nil
}
func (f *fakeArchiveRepo) Remove(_ context.Context, _, slug string) error {
	for i, e := range f.entries {
		if e.Slug == slug {
			f.entries = append(f.entries[:i], f.entries[i+1:]...)
			return nil
		}
	}
	return domain.ErrArchiveEntryNotFound
}
func (f *fakeArchiveRepo) Get(_ context.Context, _ string, slug string) (*domain.ArchiveEntry, error) {
	for _, e := range f.entries {
		if e.Slug == slug {
			return e, nil
		}
	}
	return nil, domain.ErrArchiveEntryNotFound
}

func archiveWorkspace(settings *domain.ArchiveSettings) *domain.Workspace {
	return &domain.Workspace{
		ID:       "ws-1",
		Name:     "Kois Optics",
		Settings: domain.WorkspaceSettings{Archive: settings},
	}
}

func archiveHandler(t *testing.T, ws *domain.Workspace, repo domain.CampaignArchiveRepository) *ArchiveHandler {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(ws, nil).AnyTimes()

	templateRepo := mocks.NewMockTemplateRepository(ctrl)
	return NewArchiveHandler(workspaceRepo, templateRepo, repo, nil, logger.NewLogger())
}

func get(h *ArchiveHandler, path string, auth func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if auth != nil {
		auth(req)
	}
	rec := httptest.NewRecorder()
	h.handle(rec, req)
	return rec
}

func TestArchive_ServesTheListNewestFirst(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, Title: "Kois archive"})
	repo := &fakeArchiveRepo{entries: []*domain.ArchiveEntry{
		{Slug: "older", Title: "Older campaign", SentAt: time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC)},
		{Slug: "newer", Title: "Newer campaign", SentAt: time.Date(2024, 7, 9, 0, 0, 0, 0, time.UTC)},
	}}

	rec := get(archiveHandler(t, ws, repo), "/archive/koisoptics/", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Kois archive")
	assert.Contains(t, body, "2 campaigns")
	assert.Less(t, strings.Index(body, "Newer campaign"), strings.Index(body, "Older campaign"),
		"the archive reads newest first, like the archive it replaces")
	// The path segment must be a stable slug: the workspace name has a space.
	assert.Contains(t, body, "/archive/kois-optics/newer")
}

func TestArchive_RequiresBasicAuthWhenConfigured(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{
		Enabled: true, BasicAuthUser: "reader", BasicAuthPassword: "s3cret",
	})
	repo := &fakeArchiveRepo{entries: []*domain.ArchiveEntry{{Slug: "x", Title: "X", SentAt: time.Now()}}}
	h := archiveHandler(t, ws, repo)

	rec := get(h, "/archive/koisoptics/", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Basic")
	assert.Equal(t, "noindex", rec.Header().Get("X-Robots-Tag"))

	rec = get(h, "/archive/koisoptics/", func(r *http.Request) { r.SetBasicAuth("reader", "wrong") })
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = get(h, "/archive/koisoptics/", func(r *http.Request) { r.SetBasicAuth("reader", "s3cret") })
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestArchive_IsOffUnlessTheWorkspaceEnablesIt(t *testing.T) {
	for name, settings := range map[string]*domain.ArchiveSettings{
		"nil settings": nil,
		"disabled":     {Enabled: false},
	} {
		t.Run(name, func(t *testing.T) {
			rec := get(archiveHandler(t, archiveWorkspace(settings), &fakeArchiveRepo{}), "/archive/koisoptics/", nil)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestArchive_UnknownWorkspaceIsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(nil, assert.AnError)
	workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil)

	h := NewArchiveHandler(workspaceRepo, nil, &fakeArchiveRepo{}, nil, logger.NewLogger())
	rec := get(h, "/archive/nope/", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestArchive_UnknownCampaignIsNotFound(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true})
	rec := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/missing", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestArchive_ToolbarHasTheMailchimpControls(t *testing.T) {
	// Gated, because that is how it runs today: three controls, and Translate
	// withheld until Google's proxy could actually fetch the page.
	ws := archiveWorkspace(&domain.ArchiveSettings{
		Enabled: true, BasicAuthUser: "reader", BasicAuthPassword: "s3cret",
	})
	ws.Settings.WebsiteURL = "https://www.kois-optics.gr/"
	repo := &fakeArchiveRepo{entries: []*domain.ArchiveEntry{{Slug: "x", Title: "X", SentAt: time.Now()}}}
	auth := func(r *http.Request) { r.SetBasicAuth("reader", "s3cret") }

	body := get(archiveHandler(t, ws, repo), "/archive/koisoptics/", auth).Body.String()

	for _, want := range []string{"Subscribe", "Past Issues", "RSS", "/archive/kois-optics/feed.xml"} {
		assert.Contains(t, body, want)
	}
	// Subscribe points at the workspace's own site when no explicit URL is set.
	assert.Contains(t, body, "https://www.kois-optics.gr/")
	assert.NotContains(t, body, "translate.google",
		"a translate proxy with no access to the page is a control that cannot work")
}

func TestArchive_ToolbarShowsTranslateWhenPublic(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, SubscribeURL: "https://example.test/join"})
	repo := &fakeArchiveRepo{}

	body := get(archiveHandler(t, ws, repo), "/archive/koisoptics/", nil).Body.String()

	assert.Contains(t, body, "translate.google.com")
	assert.Contains(t, body, "https://example.test/join")
}

func TestArchive_ServesRSS(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, Title: "Kois archive"})
	repo := &fakeArchiveRepo{entries: []*domain.ArchiveEntry{
		{Slug: "newer", Title: "Newer & better", Subject: "Subject", SentAt: time.Date(2024, 7, 9, 0, 0, 0, 0, time.UTC)},
		{Slug: "older", Title: "Older", SentAt: time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC)},
	}}

	rec := get(archiveHandler(t, ws, repo), "/archive/koisoptics/feed.xml", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/rss+xml")
	body := rec.Body.String()
	assert.Contains(t, body, `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">`)
	assert.Contains(t, body, "Newer &amp; better", "escaping matters: & is not valid XML")
	assert.Contains(t, body, "/archive/kois-optics/feed.xml")
	assert.Less(t, strings.Index(body, "newer"), strings.Index(body, "older"))
}

func TestArchive_FeedIsGatedToo(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, BasicAuthUser: "u", BasicAuthPassword: "p"})
	rec := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/feed.xml", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestArchive_ListPageStylesItsToolbar(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, Title: "Kois Optics · Newsletter archive"})
	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/", nil).Body.String()

	// The bar is HTML in both pages but its CSS lived only in the campaign wrapper,
	// which made the list page render four unstyled links in a row.
	assert.Contains(t, body, ".mca-bar{")
	// The bar carries the brand, not the workspace's slug-like name.
	assert.Contains(t, body, `class="mca-name" href="/archive/kois-optics/">Kois Optics<`)
}

func TestArchive_SubscribePointsAtOurFormWhenAListIsConfigured(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{
		Enabled: true, SubscribeListIDs: []string{"newsletter"},
		// An explicit external URL must lose to the list: a form we host is the
		// Mailchimp behaviour, an outbound link is the fallback.
		SubscribeURL: "www.example.gr",
	})

	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/", nil).Body.String()
	assert.Contains(t, body, `href="/archive/kois-optics/subscribe"`)
}

func TestArchive_ExternalSubscribeGetsAScheme(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, SubscribeURL: "www.kois-optics.gr"})
	ws.Settings.WebsiteURL = "kois-optics.gr"

	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/", nil).Body.String()

	// A scheme-less setting used to resolve against the archive path and land on
	// /archive/kois-optics/www.kois-optics.gr.
	assert.Contains(t, body, `href="https://www.kois-optics.gr"`)
	assert.NotContains(t, body, "/archive/kois-optics/www.")
}

func TestArchive_SubscribeFormIsServed(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, SubscribeListIDs: []string{"newsletter"}})

	rec := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/subscribe", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `<form method="post" action="/archive/kois-optics/subscribe">`)
	assert.Contains(t, body, `name="email"`)
	assert.Contains(t, body, "Subscribe to Kois Optics")
}

func TestArchive_SubscribeFormIsAbsentWithoutAList(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true})
	rec := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/subscribe", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "no list configured means no form to post to")
}

func TestArchive_SubscribeSuccessDoesNotPromiseAnEmail(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, SubscribeListIDs: []string{"newsletter"}})
	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/subscribe?ok=1", nil).Body.String()

	assert.Contains(t, body, "You are on the list.")
	// Whether a confirmation is sent depends on the list's double opt-in setting,
	// which this page does not read, so it must not claim one is coming.
	assert.NotContains(t, body, "Check your inbox")
}

func TestArchive_HomeCarriesAnInlineSignup(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, SubscribeListIDs: []string{"newsletter"}})
	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/", nil).Body.String()

	// Mailchimp's archive home offers signup in the page, not only a link: a reader
	// forwarded one campaign has arrived without ever subscribing.
	assert.Contains(t, body, `class="inline-signup"`)
	assert.Contains(t, body, `action="/archive/kois-optics/subscribe"`)
}

func TestArchive_HomeHasNoSignupWithoutAList(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true})
	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/", nil).Body.String()
	assert.NotContains(t, body, `class="inline-signup"`)
}

func TestArchive_SubscribeFormTakesBothNames(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, SubscribeListIDs: []string{"newsletter"}})
	body := get(archiveHandler(t, ws, &fakeArchiveRepo{}), "/archive/koisoptics/subscribe", nil).Body.String()

	// Mailchimp collects first and last; one field was a simplification, not a choice.
	assert.Contains(t, body, `name="first_name"`)
	assert.Contains(t, body, `name="last_name"`)
}
