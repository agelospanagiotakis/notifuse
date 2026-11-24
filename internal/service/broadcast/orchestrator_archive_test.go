package broadcast

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

func newArchiveTestOrchestrator(hook BroadcastSentHook) *BroadcastOrchestrator {
	return &BroadcastOrchestrator{
		config: &Config{OnBroadcastSent: hook},
		logger: logger.NewLoggerWithLevel("disabled"),
	}
}

func TestArchiveSentBroadcastArchivesSingleVariation(t *testing.T) {
	var gotWorkspace, gotBroadcast, gotTemplate string
	var gotVersion int64
	called := false
	o := newArchiveTestOrchestrator(func(_ context.Context, workspaceID, broadcastID, templateID string, templateVersion int64) error {
		called = true
		gotWorkspace, gotBroadcast, gotTemplate, gotVersion = workspaceID, broadcastID, templateID, templateVersion
		return nil
	})
	b := &domain.Broadcast{
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{TemplateID: "tpl-1"}},
		},
	}
	templates := map[string]*domain.Template{"tpl-1": {ID: "tpl-1", Version: 9}}

	o.archiveSentBroadcast(context.Background(), "ws-1", b, &domain.SendBroadcastState{BroadcastID: "bc-1"}, templates, 42)

	if !called {
		t.Fatal("hook was not called")
	}
	if gotWorkspace != "ws-1" || gotBroadcast != "bc-1" || gotTemplate != "tpl-1" || gotVersion != 9 {
		t.Fatalf("hook args = %q %q %q v%d", gotWorkspace, gotBroadcast, gotTemplate, gotVersion)
	}
}

func TestArchiveSentBroadcastArchivesWinner(t *testing.T) {
	var gotTemplate string
	winner := "tpl-winner"
	o := newArchiveTestOrchestrator(func(_ context.Context, _, _, templateID string, _ int64) error {
		gotTemplate = templateID
		return nil
	})
	b := &domain.Broadcast{
		WinningTemplate: &winner,
		TestSettings: domain.BroadcastTestSettings{
			Variations: []domain.BroadcastVariation{{TemplateID: "tpl-loser"}},
		},
	}

	o.archiveSentBroadcast(context.Background(), "ws", b, &domain.SendBroadcastState{BroadcastID: "bc"}, nil, 10)

	if gotTemplate != "tpl-winner" {
		t.Fatalf("archived %q, want the winning template", gotTemplate)
	}
}

func TestArchiveSentBroadcastSkipsWithoutRecipients(t *testing.T) {
	called := false
	o := newArchiveTestOrchestrator(func(context.Context, string, string, string, int64) error {
		called = true
		return nil
	})
	b := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{TemplateID: "tpl"}}}}

	o.archiveSentBroadcast(context.Background(), "ws", b, &domain.SendBroadcastState{BroadcastID: "bc"}, nil, 0)

	if called {
		t.Fatal("hook ran for a broadcast that enqueued nothing")
	}
}

func TestArchiveSentBroadcastSkipsWithoutHook(t *testing.T) {
	o := newArchiveTestOrchestrator(nil)
	b := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{TemplateID: "tpl"}}}}
	// Must not panic when no hook is configured.
	o.archiveSentBroadcast(context.Background(), "ws", b, &domain.SendBroadcastState{BroadcastID: "bc"}, nil, 5)
}

func TestArchiveSentBroadcastSwallowsHookError(t *testing.T) {
	o := newArchiveTestOrchestrator(func(context.Context, string, string, string, int64) error {
		return errors.New("archive down")
	})
	b := &domain.Broadcast{TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{TemplateID: "tpl"}}}}
	// A failing archive must never fail the send: this just returns.
	o.archiveSentBroadcast(context.Background(), "ws", b, &domain.SendBroadcastState{BroadcastID: "bc"}, nil, 5)
}
