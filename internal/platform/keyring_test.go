package platform

import (
	"context"
	"errors"
	"testing"

	"github.com/Jason-Wang1245/db-tui/internal/profile"
)

func TestKeyringHonorsCancelledContextBeforePlatformCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	keyring := NewKeyring("db-tui-test")

	if _, err := keyring.Get(ctx, profile.ID("profile")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get returned %v, want context.Canceled", err)
	}
	if err := keyring.Set(ctx, profile.ID("profile"), "secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Set returned %v, want context.Canceled", err)
	}
	if err := keyring.Delete(ctx, profile.ID("profile")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete returned %v, want context.Canceled", err)
	}
}
