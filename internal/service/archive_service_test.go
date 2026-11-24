package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestArchiveSentBroadcastBuildsEntry(t *testing.T) {
	sentAt := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	var got *domain.ArchiveEntry
	var gotWorkspace string

	s := &ArchiveService{
		loadBroadcast: func(_ context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error) {
			if workspaceID != "ws-1" || broadcastID != "bc-1" {
				t.Fatalf("unexpected lookup %q/%q", workspaceID, broadcastID)
			}
			return &domain.Broadcast{
				Name:     "Autumn Sale",
				Audience: domain.AudienceSettings{List: "newsletter"},
			}, nil
		},
		loadTemplate: func(_ context.Context, _ string, templateID string, version int64) (*domain.Template, error) {
			if templateID != "tpl-1" || version != 7 {
				t.Fatalf("unexpected template lookup %q v%d", templateID, version)
			}
			return &domain.Template{
				ID:      "tpl-1",
				Name:    "Autumn template",
				Version: 7,
				Email:   &domain.EmailTemplate{Subject: "Leaves are falling"},
			}, nil
		},
		upsert: func(_ context.Context, workspaceID string, entry *domain.ArchiveEntry) error {
			gotWorkspace = workspaceID
			got = entry
			return nil
		},
		now: fixedNow(sentAt),
	}

	if err := s.ArchiveSentBroadcast(context.Background(), "ws-1", "bc-1", "tpl-1", 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected an archive entry")
	}
	if gotWorkspace != "ws-1" {
		t.Errorf("workspace = %q", gotWorkspace)
	}
	if got.Title != "Autumn Sale" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Subject != "Leaves are falling" {
		t.Errorf("subject = %q", got.Subject)
	}
	if got.TemplateID != "tpl-1" || got.TemplateVersion != 7 {
		t.Errorf("template = %q v%d", got.TemplateID, got.TemplateVersion)
	}
	if got.ListID != "newsletter" {
		t.Errorf("list = %q", got.ListID)
	}
	if got.SentAt != sentAt {
		t.Errorf("sent_at = %v", got.SentAt)
	}
	if got.Slug != "2026-11-24-autumn-sale" {
		t.Errorf("slug = %q", got.Slug)
	}
}

func TestArchiveSentBroadcastFallsBackToTemplateName(t *testing.T) {
	var got *domain.ArchiveEntry
	s := &ArchiveService{
		loadBroadcast: func(context.Context, string, string) (*domain.Broadcast, error) {
			return &domain.Broadcast{Name: "   "}, nil
		},
		loadTemplate: func(context.Context, string, string, int64) (*domain.Template, error) {
			return &domain.Template{Name: "Fallback name"}, nil
		},
		upsert: func(_ context.Context, _ string, entry *domain.ArchiveEntry) error {
			got = entry
			return nil
		},
		now: fixedNow(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
	}

	if err := s.ArchiveSentBroadcast(context.Background(), "ws", "bc", "tpl", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Title != "Fallback name" {
		t.Fatalf("title = %#v", got)
	}
}

func TestArchiveSentBroadcastUsesTemplateIDWhenUnnamed(t *testing.T) {
	var got *domain.ArchiveEntry
	s := &ArchiveService{
		loadBroadcast: func(context.Context, string, string) (*domain.Broadcast, error) {
			return nil, errors.New("not found")
		},
		upsert: func(_ context.Context, _ string, entry *domain.ArchiveEntry) error {
			got = entry
			return nil
		},
		now: fixedNow(time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)),
	}

	if err := s.ArchiveSentBroadcast(context.Background(), "ws", "bc", "mc_abcdef123", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Title != "mc_abcdef123" {
		t.Fatalf("title = %#v", got)
	}
	if got.Slug != "2026-02-03-mc-abcdef123" {
		t.Errorf("slug = %q", got.Slug)
	}
}

func TestArchiveSentBroadcastSlugFallbackForNonLatinTitle(t *testing.T) {
	got := archiveSlugForSend(time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC), "Ωμέγα", "mc_abcdef123")
	if got != "2026-02-03-abcdef" {
		t.Errorf("slug = %q", got)
	}
}

func TestArchiveSentBroadcastNoopsWithoutIDs(t *testing.T) {
	called := false
	s := &ArchiveService{
		upsert: func(context.Context, string, *domain.ArchiveEntry) error {
			called = true
			return nil
		},
	}
	if err := s.ArchiveSentBroadcast(context.Background(), "ws", "", "tpl", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.ArchiveSentBroadcast(context.Background(), "ws", "bc", "", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected no upsert without broadcast/template id")
	}
}

func TestArchiveSentBroadcastPropagatesUpsertError(t *testing.T) {
	wantErr := errors.New("db down")
	s := &ArchiveService{
		loadBroadcast: func(context.Context, string, string) (*domain.Broadcast, error) { return nil, nil },
		upsert: func(context.Context, string, *domain.ArchiveEntry) error {
			return wantErr
		},
	}
	if err := s.ArchiveSentBroadcast(context.Background(), "ws", "bc", "tpl", 0); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v", err)
	}
}
