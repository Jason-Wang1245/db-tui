package platform

import (
	"path/filepath"
	"testing"
)

func TestConfigPathsAreDerivedFromInjectedBase(t *testing.T) {
	paths := NewConfigPaths(filepath.Join("test", "config"))
	want := filepath.Join("test", "config", "db-tui", "profiles.json")
	if got := paths.ProfilesFile(); got != want {
		t.Fatalf("ProfilesFile returned %q, want %q", got, want)
	}
}
