package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jason-Wang1245/db-tui/internal/profile"
)

func TestJSONProfileRepositoryUsesRestrictivePermissionsAndNoSecrets(t *testing.T) {
	base := t.TempDir()
	paths := NewConfigPaths(base)
	repository := NewJSONProfileRepository(paths)
	document := profile.Document{Version: profile.CurrentDocumentVersion, Profiles: []profile.Profile{{
		ID: "one", Name: "Local", Host: "localhost", Port: 5432, Database: "app", User: "developer",
		SSLMode: "prefer", SavePassword: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}}}
	if err := repository.Save(context.Background(), document); err != nil {
		t.Fatal(err)
	}
	directoryInfo, err := os.Stat(paths.ConfigDirectory())
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(paths.ProfilesFile())
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("permissions directory=%o file=%o", directoryInfo.Mode().Perm(), fileInfo.Mode().Perm())
	}
	data, err := os.ReadFile(paths.ProfilesFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"\"password\":", "connection_url", "postgresql://"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("profiles file contains forbidden data %q: %s", forbidden, data)
		}
	}
	loaded, err := repository.Load(context.Background())
	if err != nil || len(loaded.Profiles) != 1 || loaded.Profiles[0].ID != "one" {
		t.Fatalf("Load = %#v, %v", loaded, err)
	}
}

func TestJSONProfileRepositoryPreservesUnknownFields(t *testing.T) {
	base := t.TempDir()
	paths := NewConfigPaths(base)
	if err := os.MkdirAll(paths.ConfigDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	original := `{"version":1,"future":true,"profiles":[{"id":"one","name":"Local","host":"localhost","port":5432,"database":"app","user":"dev","ssl_mode":"prefer","save_password":false,"created_at":"2026-08-13T00:00:00Z","updated_at":"2026-08-13T00:00:00Z","profile_future":{"x":1}}]}`
	if err := os.WriteFile(paths.ProfilesFile(), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	repository := NewJSONProfileRepository(paths)
	document, err := repository.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document.Profiles[0].Name = "Renamed"
	if err := repository.Save(context.Background(), document); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths.ProfilesFile())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["future"]; !ok {
		t.Fatal("document future field was dropped")
	}
	var profiles []map[string]json.RawMessage
	if err := json.Unmarshal(decoded["profiles"], &profiles); err != nil {
		t.Fatal(err)
	}
	if _, ok := profiles[0]["profile_future"]; !ok {
		t.Fatal("profile future field was dropped")
	}
}

func TestJSONProfileRepositoryRejectsSymlinkAndUnsupportedVersion(t *testing.T) {
	base := t.TempDir()
	paths := NewConfigPaths(base)
	if err := os.MkdirAll(paths.ConfigDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "target.json")
	if err := os.WriteFile(target, []byte(`{"version":1,"profiles":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, paths.ProfilesFile()); err != nil {
		t.Fatal(err)
	}
	repository := NewJSONProfileRepository(paths)
	if _, err := repository.Load(context.Background()); err == nil {
		t.Fatal("symlink unexpectedly accepted")
	}
	if err := os.Remove(paths.ProfilesFile()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ProfilesFile(), []byte(`{"version":99,"profiles":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Load(context.Background()); err == nil {
		t.Fatal("unsupported version unexpectedly accepted")
	}
}

func TestJSONProfileRepositoryRejectsSymlinkedConfigDirectory(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := NewConfigPaths(base)
	if err := os.Symlink(target, paths.ConfigDirectory()); err != nil {
		t.Fatal(err)
	}
	repository := NewJSONProfileRepository(paths)
	if _, err := repository.Load(context.Background()); err == nil {
		t.Fatal("symlinked config directory unexpectedly accepted")
	}
}

func TestJSONProfileRepositoryRejectsSecretFieldsAndDuplicateIDs(t *testing.T) {
	base := t.TempDir()
	paths := NewConfigPaths(base)
	if err := os.MkdirAll(paths.ConfigDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`{"version":1,"password":"must-not-live-here","profiles":[]}`,
		`{"version":1,"profiles":[{"id":"same","name":"One","host":"localhost","port":5432,"database":"app","user":"dev","ssl_mode":"prefer","save_password":false,"created_at":"2026-08-13T00:00:00Z","updated_at":"2026-08-13T00:00:00Z"},{"id":"same","name":"Two","host":"localhost","port":5432,"database":"app","user":"dev","ssl_mode":"prefer","save_password":false,"created_at":"2026-08-13T00:00:00Z","updated_at":"2026-08-13T00:00:00Z"}]}`,
	}
	for _, data := range cases {
		if err := os.WriteFile(paths.ProfilesFile(), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewJSONProfileRepository(paths).Load(context.Background()); err == nil {
			t.Fatalf("invalid profile document unexpectedly accepted: %s", data)
		}
	}
}
