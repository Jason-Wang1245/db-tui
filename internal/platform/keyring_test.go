package platform

import (
	"context"
	"errors"
	"testing"

	systemkeyring "github.com/zalando/go-keyring"

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

func TestKeyringStoresByStableProfileID(t *testing.T) {
	systemkeyring.MockInit()
	adapter := NewKeyring("db-tui-test")
	ctx := context.Background()
	id := profile.ID("stable-profile-id")
	if err := adapter.Set(ctx, id, "secret"); err != nil {
		t.Fatal(err)
	}
	value, err := adapter.Get(ctx, id)
	if err != nil || value != "secret" {
		t.Fatalf("Get = %q, %v", value, err)
	}
	if err := adapter.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Get(ctx, id); !errors.Is(err, profile.ErrSecretNotFound) {
		t.Fatalf("Get after Delete = %v", err)
	}
}
