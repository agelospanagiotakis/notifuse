package service

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// fakeArchiveStore is the archive repository, in memory. The real one writes to the workspace
// database; everything this service decides is visible in what reaches it.
type fakeArchiveStore struct {
	entries      map[string]*domain.ArchiveEntry // by slug
	audiences    map[string]*domain.ArchiveAudience
	schemaCalled int
}

func newFakeArchiveStore() *fakeArchiveStore {
	return &fakeArchiveStore{
		entries:   map[string]*domain.ArchiveEntry{},
		audiences: map[string]*domain.ArchiveAudience{},
	}
}

func (f *fakeArchiveStore) EnsureSchema(context.Context, string) error { f.schemaCalled++; return nil }

func (f *fakeArchiveStore) List(context.Context, string) ([]*domain.ArchiveEntry, error) {
	out := make([]*domain.ArchiveEntry, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeArchiveStore) Get(_ context.Context, _ string, slug string) (*domain.ArchiveEntry, error) {
	if e, ok := f.entries[slug]; ok {
		return e, nil
	}
	return nil, domain.ErrArchiveEntryNotFound
}

func (f *fakeArchiveStore) Upsert(_ context.Context, _ string, e *domain.ArchiveEntry) error {
	clone := *e
	f.entries[e.Slug] = &clone
	return nil
}

func (f *fakeArchiveStore) Remove(_ context.Context, _ string, slug string) error {
	delete(f.entries, slug)
	return nil
}

func (f *fakeArchiveStore) ListAudiences(context.Context, string) ([]*domain.ArchiveAudience, error) {
	out := make([]*domain.ArchiveAudience, 0, len(f.audiences))
	for _, a := range f.audiences {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeArchiveStore) ListByAudienceIDs(_ context.Context, _ string, ids []string) ([]*domain.ArchiveEntry, error) {
	out := []*domain.ArchiveEntry{}
	for _, e := range f.entries {
		for _, id := range ids {
			if e.ListID == id {
				out = append(out, e)
				break
			}
		}
	}
	return out, nil
}

func (f *fakeArchiveStore) GetAudience(_ context.Context, _ string, slugOrID string) (*domain.ArchiveAudience, error) {
	for _, a := range f.audiences {
		if a.Slug == slugOrID || a.ListID == slugOrID || (a.NotifuseListID != "" && a.NotifuseListID == slugOrID) {
			return a, nil
		}
	}
	return nil, domain.ErrArchiveAudienceNotFound
}

func (f *fakeArchiveStore) UpsertAudience(_ context.Context, _ string, a *domain.ArchiveAudience) error {
	clone := *a
	f.audiences[a.ListID] = &clone
	return nil
}

func (f *fakeArchiveStore) SetAudienceListed(_ context.Context, _ string, listID string, listed bool) error {
	if a, ok := f.audiences[listID]; ok {
		a.Listed = listed
		return nil
	}
	return domain.ErrArchiveAudienceNotFound
}

func (f *fakeArchiveStore) GetByBroadcastID(_ context.Context, _ string, broadcastID string) (*domain.ArchiveEntry, error) {
	for _, e := range f.entries {
		if e.BroadcastID == broadcastID {
			return e, nil
		}
	}
	return nil, domain.ErrArchiveEntryNotFound
}

func (f *fakeArchiveStore) SlugExists(_ context.Context, _ string, slug string) (bool, error) {
	_, ok := f.entries[slug]
	return ok, nil
}

type archiveHarness struct {
	svc       *ArchiveService
	store     *fakeArchiveStore
	broadcast *mocks.MockBroadcastRepository
	templates *mocks.MockTemplateRepository
	lists     *mocks.MockListRepository
}

func newArchiveHarness(t *testing.T) *archiveHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	store := newFakeArchiveStore()
	broadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	templateRepo := mocks.NewMockTemplateRepository(ctrl)
	listRepo := mocks.NewMockListRepository(ctrl)

	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()

	return &archiveHarness{
		svc:       NewArchiveService(broadcastRepo, templateRepo, listRepo, store, log),
		store:     store,
		broadcast: broadcastRepo,
		templates: templateRepo,
		lists:     listRepo,
	}
}

func sentBroadcast() *domain.Broadcast {
	started := time.Date(2026, 11, 4, 9, 30, 0, 0, time.UTC)
	completed := started.Add(4 * time.Minute)
	return &domain.Broadcast{
		ID:        "bc-1",
		Name:      "Προσφορά Νοεμβρίου",
		Status:    domain.BroadcastStatusProcessed,
		StartedAt: &started,
		CompletedAt: &completed,
		Audience:  domain.AudienceSettings{List: "koisopticsmain"},
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{VariationName: "default", TemplateID: "example-offer"}},
		},
		EnqueuedCount: 11924,
	}
}

func TestArchiveService_ArchivesASentCampaign(t *testing.T) {
	ctx := context.Background()
	h := newArchiveHarness(t)
	b := sentBroadcast()

	h.broadcast.EXPECT().GetBroadcast(ctx, "koisoptics", "bc-1").Return(b, nil)
	tpl := &domain.Template{ID: "example-offer", Version: 3, Email: &domain.EmailTemplate{Subject: "Προσφορά"}}
	h.templates.EXPECT().GetTemplateByID(ctx, "koisoptics", "example-offer", int64(0)).Return(tpl, nil).AnyTimes()
	h.lists.EXPECT().GetListByID(ctx, "koisoptics", "koisopticsmain").Return(&domain.List{ID: "koisopticsmain", Name: "Kois Optics"}, nil)

	require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))

	require.Len(t, h.store.entries, 1)
	var entry *domain.ArchiveEntry
	for _, e := range h.store.entries {
		entry = e
	}
	// The date is the send date, not the archive date: a 2014 campaign must say 2014.
	assert.Equal(t, "2026-11-04-prosfora-noemvrioy", entry.Slug)
	assert.Equal(t, "example-offer", entry.TemplateID)
	assert.Equal(t, int64(3), entry.TemplateVersion, "the version that was sent")
	assert.Equal(t, "koisopticsmain", entry.ListID)
	assert.Equal(t, "bc-1", entry.BroadcastID)
	assert.Equal(t, b.StartedAt.UTC(), entry.SentAt.UTC())
	assert.Equal(t, "Προσφορά", entry.Subject)

	// The audience has to exist before the campaign's page can resolve.
	aud, ok := h.store.audiences["koisopticsmain"]
	require.True(t, ok, "the platform list must be registered as an archive audience")
	assert.Equal(t, "kois-optics", aud.Slug)
	assert.Equal(t, "Kois Optics", aud.Name)
	assert.Equal(t, "koisopticsmain", aud.NotifuseListID)
}

func TestArchiveService_SkipsWhatWasNotSent(t *testing.T) {
	ctx := context.Background()

	t.Run("a broadcast that is not processed", func(t *testing.T) {
		h := newArchiveHarness(t)
		b := sentBroadcast()
		b.Status = domain.BroadcastStatusPaused
		h.broadcast.EXPECT().GetBroadcast(ctx, gomock.Any(), gomock.Any()).Return(b, nil)

		require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))
		assert.Empty(t, h.store.entries)
	})

	t.Run("a cancelled broadcast", func(t *testing.T) {
		h := newArchiveHarness(t)
		b := sentBroadcast()
		now := time.Now().UTC()
		b.CancelledAt = &now
		h.broadcast.EXPECT().GetBroadcast(ctx, gomock.Any(), gomock.Any()).Return(b, nil)

		require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))
		assert.Empty(t, h.store.entries)
	})

	t.Run("a broadcast with no audience", func(t *testing.T) {
		h := newArchiveHarness(t)
		b := sentBroadcast()
		b.Audience = domain.AudienceSettings{}
		h.broadcast.EXPECT().GetBroadcast(ctx, gomock.Any(), gomock.Any()).Return(b, nil)

		require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))
		assert.Empty(t, h.store.entries)
	})
}

func TestArchiveService_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	h := newArchiveHarness(t)
	b := sentBroadcast()

	h.broadcast.EXPECT().GetBroadcast(ctx, "koisoptics", "bc-1").Return(b, nil).Times(2)
	tpl := &domain.Template{ID: "example-offer", Version: 3, Email: &domain.EmailTemplate{Subject: "s"}}
	h.templates.EXPECT().GetTemplateByID(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(tpl, nil).AnyTimes()
	h.lists.EXPECT().GetListByID(ctx, gomock.Any(), gomock.Any()).Return(&domain.List{ID: "koisopticsmain", Name: "Kois Optics"}, nil).AnyTimes()

	require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))
	require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))

	assert.Len(t, h.store.entries, 1, "a retried task must not publish the campaign twice")
}

func TestArchiveService_SlugCollision(t *testing.T) {
	ctx := context.Background()
	h := newArchiveHarness(t)
	b := sentBroadcast()

	h.broadcast.EXPECT().GetBroadcast(ctx, "koisoptics", "bc-1").Return(b, nil)
	tpl := &domain.Template{ID: "example-offer", Version: 3, Email: &domain.EmailTemplate{Subject: "s"}}
	h.templates.EXPECT().GetTemplateByID(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(tpl, nil).AnyTimes()
	h.lists.EXPECT().GetListByID(ctx, gomock.Any(), gomock.Any()).Return(&domain.List{ID: "koisopticsmain", Name: "Kois Optics"}, nil)

	// Another campaign already took the readable slug.
	h.store.entries["2026-11-04-prosfora-noemvrioy"] = &domain.ArchiveEntry{Slug: "2026-11-04-prosfora-noemvrioy"}

	require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 3))

	_, taken := h.store.entries["2026-11-04-prosfora-noemvrioy-bc-1"]
	assert.True(t, taken, "a collision must not overwrite another campaign's page")
	assert.Len(t, h.store.entries, 2)
}

func TestArchiveService_ReusesAnExistingAudience(t *testing.T) {
	ctx := context.Background()
	h := newArchiveHarness(t)
	b := sentBroadcast()

	// The imported audience, already registered with its legacy id.
	h.store.audiences["d545c36792"] = &domain.ArchiveAudience{
		ListID: "d545c36792", Slug: "kois-optics", Name: "Kois Optics",
		NotifuseListID: "koisopticsmain", Listed: true,
	}

	h.broadcast.EXPECT().GetBroadcast(ctx, gomock.Any(), gomock.Any()).Return(b, nil)
	tpl := &domain.Template{ID: "example-offer", Version: 1, Email: &domain.EmailTemplate{}}
	h.templates.EXPECT().GetTemplateByID(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(tpl, nil).AnyTimes()

	require.NoError(t, h.svc.ArchiveSentBroadcast(ctx, "koisoptics", "bc-1", "example-offer", 1))

	assert.Len(t, h.store.audiences, 1, "a second row for the same audience would split its page in two")
}
