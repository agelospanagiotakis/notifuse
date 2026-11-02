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
	return NewArchiveHandler(workspaceRepo, templateRepo, repo, logger.NewLogger())
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

	h := NewArchiveHandler(workspaceRepo, nil, &fakeArchiveRepo{}, logger.NewLogger())
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
