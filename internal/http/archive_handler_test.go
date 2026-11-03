package http

import (
	"context"
	"errors"
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
	entries   []*domain.ArchiveEntry
	audiences []*domain.ArchiveAudience
	schema    int
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

func (f *fakeArchiveRepo) ListAudiences(context.Context, string) ([]*domain.ArchiveAudience, error) {
	if len(f.audiences) > 0 {
		return f.audiences, nil
	}
	// Derived from the entries, so a test that only seeds campaigns still gets a nav: the
	// real repository groups the same way.
	counts := map[string]int{}
	var order []string
	for _, e := range f.entries {
		if e.ListID == "" {
			continue
		}
		if _, seen := counts[e.ListID]; !seen {
			order = append(order, e.ListID)
		}
		counts[e.ListID]++
	}
	out := make([]*domain.ArchiveAudience, 0, len(order))
	for _, id := range order {
		out = append(out, &domain.ArchiveAudience{
			ListID: id, Slug: id, Name: id, Campaigns: counts[id], Listed: true,
		})
	}
	return out, nil
}

func (f *fakeArchiveRepo) ListByListID(_ context.Context, _ string, listID string) ([]*domain.ArchiveEntry, error) {
	out := make([]*domain.ArchiveEntry, 0, len(f.entries))
	for _, e := range f.entries {
		if e.ListID == listID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeArchiveRepo) GetAudience(_ context.Context, _ string, slug string) (*domain.ArchiveAudience, error) {
	audiences, _ := f.ListAudiences(context.Background(), "")
	for _, a := range audiences {
		if a.Slug == slug || a.ListID == slug {
			return a, nil
		}
	}
	return nil, domain.ErrArchiveAudienceNotFound
}

func (f *fakeArchiveRepo) UpsertAudience(_ context.Context, _ string, a *domain.ArchiveAudience) error {
	f.audiences = append(f.audiences, a)
	return nil
}

func (f *fakeArchiveRepo) ListByAudienceIDs(ctx context.Context, ws string, listIDs []string) ([]*domain.ArchiveEntry, error) {
	out := make([]*domain.ArchiveEntry, 0, len(f.entries))
	for _, e := range f.entries {
		for _, id := range listIDs {
			if e.ListID == id {
				out = append(out, e)
				break
			}
		}
	}
	return out, nil
}

func (f *fakeArchiveRepo) GetByBroadcastID(_ context.Context, _ string, broadcastID string) (*domain.ArchiveEntry, error) {
	for _, e := range f.entries {
		if e.BroadcastID == broadcastID {
			return e, nil
		}
	}
	return nil, domain.ErrArchiveEntryNotFound
}

func (f *fakeArchiveRepo) SlugExists(_ context.Context, _ string, slug string) (bool, error) {
	for _, e := range f.entries {
		if e.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeArchiveRepo) SetAudienceListed(_ context.Context, _ string, listID string, listed bool) error {
	for _, a := range f.audiences {
		if a.ListID == listID {
			a.Listed = listed
			return nil
		}
	}
	return domain.ErrArchiveAudienceNotFound
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

// A campaign and an audience are both one segment under the workspace, so the two must be told
// apart by what exists rather than by the shape of the path.
func TestArchive_AudiencePages(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{Enabled: true, Title: "Kois archive"})
	repo := &fakeArchiveRepo{
		entries: []*domain.ArchiveEntry{
			{Slug: "2021-06-05-june", Title: "June 2021", SentAt: time.Date(2021, 6, 5, 0, 0, 0, 0, time.UTC), ListID: "kois"},
			{Slug: "2021-07-01-syros", Title: "Syros July", SentAt: time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC), ListID: "syros"},
			{Slug: "2022-01-01-june", Title: "January 2022", SentAt: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), ListID: "kois"},
		},
		audiences: []*domain.ArchiveAudience{
			{ListID: "kois", Slug: "kois", Name: "Kois Optics", Campaigns: 2, Listed: true},
			{ListID: "syros", Slug: "syros", Name: "SYROS", Campaigns: 1, Listed: true},
			// Unticked: must not appear in the switch, and its page must not resolve.
			{ListID: "turkey", Slug: "turkey", Name: "Turkey", Campaigns: 1, Listed: false},
		},
	}
	h := archiveHandler(t, ws, repo)

	t.Run("one audience's home lists only its campaigns", func(t *testing.T) {
		rec := get(h, "/archive/koisoptics/syros/", nil)

		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "Syros July")
		assert.NotContains(t, body, "June 2021")
		assert.NotContains(t, body, "January 2022")
		// The heading names the audience, not the workspace's generic archive title.
		assert.Contains(t, body, "SYROS")
	})

	t.Run("the audience feed carries only its campaigns", func(t *testing.T) {
		rec := get(h, "/archive/koisoptics/syros/feed.xml", nil)

		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		// Campaign pages are not duplicated per audience, so an item links to the campaign's
		// own URL; what makes this the audience's feed is which campaigns it contains.
		assert.Contains(t, body, "/archive/kois-optics/2021-07-01-syros")
		assert.NotContains(t, body, "2021-06-05-june")
		// Its own self link, so a copied feed URL is re-discovered as the audience's feed.
		assert.Contains(t, body, "/archive/kois-optics/syros/feed.xml")
	})

	t.Run("a campaign slug is not shadowed by an audience", func(t *testing.T) {
		// One segment under the workspace can be either. The campaign is tried first: this
		// request names a real entry, and its template is reported missing, so 410 proves the
		// campaign branch ran rather than an audience page being rendered instead.
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(ws, nil).AnyTimes()
		templateRepo := mocks.NewMockTemplateRepository(ctrl)
		templateRepo.EXPECT().GetTemplateByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("template gone")).AnyTimes()

		rec := get(NewArchiveHandler(workspaceRepo, templateRepo, repo, nil, logger.NewLogger()),
			"/archive/koisoptics/2021-06-05-june", nil)

		assert.Equal(t, http.StatusGone, rec.Code)
	})

	t.Run("the audience switch is absent unless the setting turns it on", func(t *testing.T) {
		// The default: which list a campaign went to is running information, and a reader on
		// an archive page is not an operator.
		rec := get(h, "/archive/koisoptics/", nil)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), `class="audiences"`)
	})

	t.Run("the nav lists every audience with its count when enabled", func(t *testing.T) {
		enabled := archiveWorkspace(&domain.ArchiveSettings{
			Enabled:         true,
			Title:           "Kois archive",
			ShowAudienceNav: true,
		})
		rec := get(archiveHandler(t, enabled, repo), "/archive/koisoptics/", nil)

		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, `class="audiences"`)
		assert.Contains(t, body, `href="/archive/kois-optics/kois/"`)
		assert.Contains(t, body, `href="/archive/kois-optics/syros/"`)
		assert.Contains(t, body, "All issues")
	})

	t.Run("an unknown audience is a 404, not an empty page", func(t *testing.T) {
		rec := get(h, "/archive/koisoptics/no-such-list/", nil)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// "Not listed" is a visibility decision, so an unticked audience must not resolve — hiding the
// link while leaving the page up would be a lock on the door and none on the window.
func TestArchive_UnlistedAudienceIsNotPublic(t *testing.T) {
	ws := archiveWorkspace(&domain.ArchiveSettings{
		Enabled:         true,
		Title:           "Kois archive",
		ShowAudienceNav: true,
	})
	repo := &fakeArchiveRepo{
		entries: []*domain.ArchiveEntry{
			{Slug: "2021-06-05-june", Title: "June 2021", SentAt: time.Date(2021, 6, 5, 0, 0, 0, 0, time.UTC), ListID: "kois"},
			{Slug: "2021-07-01-syros", Title: "Syros July", SentAt: time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC), ListID: "syros"},
			{Slug: "2016-12-21-turkey", Title: "Turkey", SentAt: time.Date(2016, 12, 21, 0, 0, 0, 0, time.UTC), ListID: "turkey"},
		},
		audiences: []*domain.ArchiveAudience{
			{ListID: "kois", Slug: "kois", Name: "Kois Optics", Campaigns: 1, Listed: true},
			{ListID: "syros", Slug: "syros", Name: "SYROS", Campaigns: 1, Listed: true},
			{ListID: "turkey", Slug: "turkey", Name: "Turkey", Campaigns: 1, Listed: false},
		},
	}
	h := archiveHandler(t, ws, repo)

	rec := get(h, "/archive/koisoptics/turkey/", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = get(h, "/archive/koisoptics/turkey/feed.xml", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Still in the archive — just not published to it.
	assert.Equal(t, 3, len(repo.entries))

	rec = get(h, "/archive/koisoptics/", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "/archive/kois-optics/syros/")
	assert.NotContains(t, body, "/archive/kois-optics/turkey/")
}
