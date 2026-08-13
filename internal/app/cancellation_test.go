package app

import (
	"context"
	"testing"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

func TestCancellationRegistryOwnsOperationCancellation(t *testing.T) {
	registry := NewCancellationRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	registry.Register(core.OperationID("query"), cancel)

	if !registry.Cancel(core.OperationID("query")) {
		t.Fatal("registered operation was not cancelled")
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("context error is %v, want context.Canceled", ctx.Err())
	}
	if registry.Cancel(core.OperationID("query")) {
		t.Fatal("operation remained registered after cancellation")
	}
}

func TestCancellationRegistryCancelsWorkspaceScope(t *testing.T) {
	registry := NewCancellationRegistry()
	contexts := make([]context.Context, 0, 2)
	for _, id := range []core.OperationID{"load", "execute"} {
		ctx, cancel := context.WithCancel(context.Background())
		contexts = append(contexts, ctx)
		registry.Register(id, cancel)
	}

	if got := registry.CancelAll(); got != 2 {
		t.Fatalf("cancelled %d operations, want 2", got)
	}
	for _, ctx := range contexts {
		if ctx.Err() != context.Canceled {
			t.Fatalf("context error is %v, want context.Canceled", ctx.Err())
		}
	}
}
