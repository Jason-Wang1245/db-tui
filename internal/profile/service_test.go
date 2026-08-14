package profile

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	document Document
	saveErr  error
}

func (repository *fakeRepository) Load(context.Context) (Document, error) {
	document := repository.document
	document.Profiles = append([]Profile(nil), document.Profiles...)
	return document, nil
}

func (repository *fakeRepository) Save(_ context.Context, document Document) error {
	if repository.saveErr != nil {
		return repository.saveErr
	}
	repository.document = document
	repository.document.Profiles = append([]Profile(nil), document.Profiles...)
	return nil
}

type fakeSecretStore struct {
	values    map[ID]string
	getErr    error
	setErr    error
	deleteErr error
}

func (store *fakeSecretStore) Get(_ context.Context, id ID) (string, error) {
	if store.getErr != nil {
		return "", store.getErr
	}
	value, ok := store.values[id]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (store *fakeSecretStore) Set(_ context.Context, id ID, value string) error {
	if store.setErr != nil {
		return store.setErr
	}
	store.values[id] = value
	return nil
}

func (store *fakeSecretStore) Delete(_ context.Context, id ID) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}
	delete(store.values, id)
	return nil
}

type fakeClock struct{ value time.Time }

func (clock fakeClock) Now() time.Time { return clock.value }

type fakeIDs struct{ value ID }

func (ids fakeIDs) NewID() (ID, error) { return ids.value, nil }

func newTestService(repository *fakeRepository, secrets *fakeSecretStore, session *SessionSecrets) *Service {
	return NewService(
		repository,
		secrets,
		session,
		fakeClock{value: time.Date(2026, 8, 13, 14, 30, 0, 0, time.FixedZone("EDT", -4*60*60))},
		fakeIDs{value: "generated-id"},
	)
}

func TestSaveStoresPasswordByStableID(t *testing.T) {
	repository := &fakeRepository{document: Document{Version: CurrentDocumentVersion}}
	secrets := &fakeSecretStore{values: make(map[ID]string)}
	session := NewSessionSecrets()
	service := newTestService(repository, secrets, session)
	draft := validDraft()
	draft.Password = "secret"
	draft.ReplacePassword = true

	result, err := service.Save(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile.ID != "generated-id" || secrets.values["generated-id"] != "secret" {
		t.Fatalf("result = %#v, secrets = %#v", result, secrets.values)
	}
	if result.CredentialMode != CredentialKeychain || !result.Profile.SavePassword {
		t.Fatalf("credential result = %#v", result)
	}
	if !result.Profile.CreatedAt.Equal(time.Date(2026, 8, 13, 18, 30, 0, 0, time.UTC)) {
		t.Fatalf("created at = %v", result.Profile.CreatedAt)
	}
}

func TestDraftDoesNotClaimMissingKeychainEntryIsStored(t *testing.T) {
	saved := Profile{ID: "existing", Name: "Local", SavePassword: true}
	repository := &fakeRepository{document: Document{Version: CurrentDocumentVersion, Profiles: []Profile{saved}}}
	service := newTestService(repository, &fakeSecretStore{values: make(map[ID]string)}, NewSessionSecrets())
	draft, err := service.Draft(context.Background(), "existing")
	if err != nil {
		t.Fatal(err)
	}
	if draft.HasStoredPassword {
		t.Fatal("draft claims a missing keychain entry is stored")
	}
}

func TestSaveFallsBackToSessionWhenKeychainUnavailable(t *testing.T) {
	repository := &fakeRepository{document: Document{Version: CurrentDocumentVersion}}
	secrets := &fakeSecretStore{values: make(map[ID]string), setErr: errors.New("locked")}
	session := NewSessionSecrets()
	service := newTestService(repository, secrets, session)
	draft := validDraft()
	draft.Password = "session-secret"
	draft.ReplacePassword = true

	result, err := service.Save(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning == nil || result.Warning.Category != "keychain" || result.Profile.SavePassword {
		t.Fatalf("fallback result = %#v", result)
	}
	if value, ok := session.Get("generated-id"); !ok || value != "session-secret" {
		t.Fatalf("session secret = %q, %v", value, ok)
	}
	if repository.document.Profiles[0].SavePassword {
		t.Fatal("profile still claims a persisted password")
	}
}

func TestReplacingExistingPasswordFallsBackWhenKeychainCannotBeRead(t *testing.T) {
	saved := Profile{
		ID: "existing", Name: "Local", Host: "localhost", Port: 5432, Database: "app",
		User: "developer", SSLMode: "prefer", SavePassword: true,
	}
	repository := &fakeRepository{document: Document{Version: CurrentDocumentVersion, Profiles: []Profile{saved}}}
	secrets := &fakeSecretStore{values: make(map[ID]string), getErr: errors.New("locked")}
	session := NewSessionSecrets()
	service := newTestService(repository, secrets, session)
	draft := DraftFromProfile(saved, true)
	draft.Password = "replacement"
	draft.ReplacePassword = true

	result, err := service.Save(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning == nil || result.Profile.SavePassword || result.CredentialMode != CredentialSession {
		t.Fatalf("fallback result = %#v", result)
	}
	if password, ok := session.Get("existing"); !ok || password != "replacement" {
		t.Fatalf("session password = %q, %v", password, ok)
	}
}

func TestTurningOffPasswordSavingDeletesKeychainEntryAndKeepsSessionCopy(t *testing.T) {
	saved := Profile{
		ID: "existing", Name: "Local", Host: "localhost", Port: 5432, Database: "app",
		User: "developer", SSLMode: "prefer", SavePassword: true,
	}
	repository := &fakeRepository{document: Document{Version: CurrentDocumentVersion, Profiles: []Profile{saved}}}
	secrets := &fakeSecretStore{values: map[ID]string{"existing": "old-secret"}}
	session := NewSessionSecrets()
	service := newTestService(repository, secrets, session)
	draft := DraftFromProfile(saved, true)
	draft.SavePassword = false

	result, err := service.Save(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := secrets.values["existing"]; ok {
		t.Fatal("keychain entry was not deleted")
	}
	if value, ok := session.Get("existing"); !ok || value != "old-secret" {
		t.Fatalf("session secret = %q, %v", value, ok)
	}
	if result.CredentialMode != CredentialSession || result.Profile.SavePassword {
		t.Fatalf("result = %#v", result)
	}
}

func TestPersistenceFailureRollsBackReplacedSecret(t *testing.T) {
	saved := Profile{
		ID: "existing", Name: "Local", Host: "localhost", Port: 5432, Database: "app",
		User: "developer", SSLMode: "prefer", SavePassword: true,
	}
	repository := &fakeRepository{
		document: Document{Version: CurrentDocumentVersion, Profiles: []Profile{saved}},
		saveErr:  errors.New("disk full"),
	}
	secrets := &fakeSecretStore{values: map[ID]string{"existing": "old-secret"}}
	service := newTestService(repository, secrets, NewSessionSecrets())
	draft := DraftFromProfile(saved, true)
	draft.Password = "new-secret"
	draft.ReplacePassword = true

	if _, err := service.Save(context.Background(), draft); err == nil {
		t.Fatal("Save unexpectedly succeeded")
	}
	if got := secrets.values["existing"]; got != "old-secret" {
		t.Fatalf("keychain rollback restored %q", got)
	}
}

func TestDeleteRemovesSecretAndRestoresItIfPersistenceFails(t *testing.T) {
	saved := Profile{ID: "existing", Name: "Local", SavePassword: true}
	repository := &fakeRepository{
		document: Document{Version: CurrentDocumentVersion, Profiles: []Profile{saved}},
		saveErr:  errors.New("disk full"),
	}
	secrets := &fakeSecretStore{values: map[ID]string{"existing": "old-secret"}}
	service := newTestService(repository, secrets, NewSessionSecrets())

	if err := service.Delete(context.Background(), "existing"); err == nil {
		t.Fatal("Delete unexpectedly succeeded")
	}
	if got := secrets.values["existing"]; got != "old-secret" {
		t.Fatalf("secret after rollback = %q", got)
	}
	repository.saveErr = nil
	if err := service.Delete(context.Background(), "existing"); err != nil {
		t.Fatal(err)
	}
	if _, ok := secrets.values["existing"]; ok || len(repository.document.Profiles) != 0 {
		t.Fatalf("delete left state behind: secrets=%#v profiles=%#v", secrets.values, repository.document.Profiles)
	}
}
