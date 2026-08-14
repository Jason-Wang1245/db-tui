package profile

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/Jason-Wang1245/db-tui/internal/core"
)

type CredentialSource string

const (
	CredentialNone     CredentialSource = "none"
	CredentialInput    CredentialSource = "input"
	CredentialKeychain CredentialSource = "keychain"
	CredentialSession  CredentialSource = "session"
)

type Prepared struct {
	Profile          Profile
	Password         string
	CredentialSource CredentialSource
	Warning          *core.Error

	draft         Draft
	document      Document
	previousIndex int
}

type SaveResult struct {
	Profile        Profile
	CredentialMode CredentialSource
	Warning        *core.Error
}

type Service struct {
	repository Repository
	secrets    SecretStore
	session    *SessionSecrets
	clock      Clock
	ids        IDGenerator
}

func NewService(repository Repository, secrets SecretStore, session *SessionSecrets, clock Clock, ids IDGenerator) *Service {
	if session == nil {
		session = NewSessionSecrets()
	}
	return &Service{repository: repository, secrets: secrets, session: session, clock: clock, ids: ids}
}

func (service *Service) Load(ctx context.Context) (Document, error) {
	document, err := service.repository.Load(ctx)
	if err != nil {
		return Document{}, err
	}
	sort.SliceStable(document.Profiles, func(left, right int) bool {
		leftTime := document.Profiles[left].LastUsedAt
		rightTime := document.Profiles[right].LastUsedAt
		if leftTime.Equal(rightTime) {
			return strings.ToLower(document.Profiles[left].Name) < strings.ToLower(document.Profiles[right].Name)
		}
		return leftTime.After(rightTime)
	})
	return document, nil
}

func (service *Service) Draft(ctx context.Context, id ID) (Draft, error) {
	document, err := service.repository.Load(ctx)
	if err != nil {
		return Draft{}, err
	}
	for _, saved := range document.Profiles {
		if saved.ID == id {
			hasStoredPassword := saved.SavePassword
			if saved.SavePassword {
				_, secretErr := service.secrets.Get(ctx, saved.ID)
				if errors.Is(secretErr, ErrSecretNotFound) {
					hasStoredPassword = false
				}
			}
			return DraftFromProfile(saved, hasStoredPassword), nil
		}
	}
	return Draft{}, core.NewError("edit profile", core.ErrorValidation, "the selected profile no longer exists", false, nil)
}

func (service *Service) Prepare(ctx context.Context, draft Draft) (Prepared, error) {
	document, err := service.repository.Load(ctx)
	if err != nil {
		return Prepared{}, err
	}
	previousIndex := -1
	for index := range document.Profiles {
		if document.Profiles[index].ID == draft.ID {
			previousIndex = index
			break
		}
	}
	if draft.ID != "" && previousIndex == -1 {
		return Prepared{}, core.NewError("prepare profile", core.ErrorValidation, "the profile was deleted by another operation", false, nil)
	}
	saved, err := ValidateDraft(draft, document)
	if err != nil {
		return Prepared{}, err
	}
	if saved.ID == "" {
		saved.ID, err = service.ids.NewID()
		if err != nil {
			return Prepared{}, core.NewError("prepare profile", core.ErrorInternal, "could not create a profile identifier", true, err)
		}
		draft.ID = saved.ID
	}
	if previousIndex >= 0 {
		previous := document.Profiles[previousIndex]
		saved.CreatedAt = previous.CreatedAt
		saved.UpdatedAt = previous.UpdatedAt
		saved.LastUsedAt = previous.LastUsedAt
		saved.Unknown = cloneRawMessages(previous.Unknown)
	}

	prepared := Prepared{Profile: saved, draft: draft, document: document, previousIndex: previousIndex}
	explicitCredential := draft.ReplacePassword || (previousIndex == -1 && draft.Password != "")
	if explicitCredential {
		prepared.Password = draft.Password
		prepared.CredentialSource = CredentialInput
		return prepared, nil
	}
	if previousIndex == -1 {
		prepared.CredentialSource = CredentialNone
		return prepared, nil
	}
	previous := document.Profiles[previousIndex]
	if previous.SavePassword {
		password, getErr := service.secrets.Get(ctx, previous.ID)
		switch {
		case getErr == nil:
			prepared.Password = password
			prepared.CredentialSource = CredentialKeychain
			return prepared, nil
		case errors.Is(getErr, ErrSecretNotFound):
			// A missing entry can be recovered from the process-local fallback.
		default:
			if password, ok := service.session.Get(previous.ID); ok {
				prepared.Password = password
				prepared.CredentialSource = CredentialSession
				prepared.Warning = keychainWarning("The system keychain is unavailable; using the session-only password.", getErr)
				return prepared, nil
			}
			return Prepared{}, keychainError("read password", "The system keychain is unavailable. Unlock it or replace the password for this connection.", getErr)
		}
	}
	if password, ok := service.session.Get(previous.ID); ok {
		prepared.Password = password
		prepared.CredentialSource = CredentialSession
		return prepared, nil
	}
	prepared.CredentialSource = CredentialNone
	return prepared, nil
}

func (service *Service) Save(ctx context.Context, draft Draft) (SaveResult, error) {
	prepared, err := service.Prepare(ctx, draft)
	if err != nil {
		return SaveResult{}, err
	}
	return service.Commit(ctx, prepared, false)
}

func (service *Service) Commit(ctx context.Context, prepared Prepared, markLastUsed bool) (SaveResult, error) {
	if err := ctx.Err(); err != nil {
		return SaveResult{}, err
	}
	if prepared.Profile.ID == "" || prepared.draft.ID != prepared.Profile.ID {
		return SaveResult{}, core.NewError("save profile", core.ErrorInternal, "the prepared profile is invalid", false, nil)
	}

	oldSession, hadOldSession := service.session.Get(prepared.Profile.ID)
	previousSavedPassword := false
	if prepared.previousIndex >= 0 {
		previousSavedPassword = prepared.document.Profiles[prepared.previousIndex].SavePassword
	}

	shouldWriteSecret := prepared.Profile.SavePassword && (prepared.draft.ReplacePassword ||
		(prepared.previousIndex == -1 && prepared.CredentialSource == CredentialInput) ||
		(!previousSavedPassword && prepared.CredentialSource == CredentialSession))
	shouldDeleteSecret := !prepared.Profile.SavePassword && prepared.previousIndex >= 0

	var oldSecret string
	oldSecretFound := false
	var keychainWriteUnavailable error
	if (shouldWriteSecret || shouldDeleteSecret) && prepared.previousIndex >= 0 {
		var getErr error
		oldSecret, getErr = service.secrets.Get(ctx, prepared.Profile.ID)
		if getErr == nil {
			oldSecretFound = true
		} else if !errors.Is(getErr, ErrSecretNotFound) {
			if shouldDeleteSecret {
				return SaveResult{}, keychainError("update password", "The existing password could not be safely removed because the system keychain is unavailable.", getErr)
			}
			keychainWriteUnavailable = getErr
		}
	}

	keychainMutated := false
	credentialMode := prepared.CredentialSource
	warning := prepared.Warning
	if shouldWriteSecret {
		if keychainWriteUnavailable != nil {
			prepared.Profile.SavePassword = false
			service.session.Set(prepared.Profile.ID, prepared.Password)
			credentialMode = CredentialSession
			warning = keychainWarning("The new password is session-only. The keychain could not be accessed, so unlock it and save again to replace or remove any previous entry.", keychainWriteUnavailable)
		} else if err := service.secrets.Set(ctx, prepared.Profile.ID, prepared.Password); err != nil {
			prepared.Profile.SavePassword = false
			service.session.Set(prepared.Profile.ID, prepared.Password)
			credentialMode = CredentialSession
			warning = keychainWarning("The profile was saved, but its password is available only for this session because the system keychain is unavailable.", err)
		} else {
			keychainMutated = true
			service.session.Delete(prepared.Profile.ID)
			credentialMode = CredentialKeychain
		}
	} else if shouldDeleteSecret {
		if err := service.secrets.Delete(ctx, prepared.Profile.ID); err != nil && !errors.Is(err, ErrSecretNotFound) {
			return SaveResult{}, keychainError("remove password", "The saved password could not be removed from the system keychain.", err)
		}
		keychainMutated = true
		if prepared.CredentialSource != CredentialNone {
			service.session.Set(prepared.Profile.ID, prepared.Password)
			credentialMode = CredentialSession
		} else {
			service.session.Delete(prepared.Profile.ID)
			credentialMode = CredentialNone
		}
	} else if !prepared.Profile.SavePassword {
		if prepared.CredentialSource != CredentialNone {
			service.session.Set(prepared.Profile.ID, prepared.Password)
			credentialMode = CredentialSession
		} else {
			credentialMode = CredentialNone
		}
	} else if previousSavedPassword || prepared.CredentialSource == CredentialKeychain {
		credentialMode = CredentialKeychain
	} else if prepared.CredentialSource == CredentialSession {
		credentialMode = CredentialSession
	} else {
		credentialMode = CredentialNone
	}

	now := service.clock.Now().UTC()
	if prepared.Profile.CreatedAt.IsZero() {
		prepared.Profile.CreatedAt = now
	}
	prepared.Profile.UpdatedAt = now
	if markLastUsed {
		prepared.Profile.LastUsedAt = now
	}
	document := prepared.document
	if document.Version == 0 {
		document.Version = CurrentDocumentVersion
	}
	if prepared.previousIndex >= 0 {
		document.Profiles[prepared.previousIndex] = prepared.Profile
	} else {
		document.Profiles = append(document.Profiles, prepared.Profile)
	}
	if err := service.repository.Save(ctx, document); err != nil {
		if keychainMutated {
			service.restoreSecret(context.WithoutCancel(ctx), prepared.Profile.ID, oldSecret, oldSecretFound)
		}
		if hadOldSession {
			service.session.Set(prepared.Profile.ID, oldSession)
		} else {
			service.session.Delete(prepared.Profile.ID)
		}
		return SaveResult{}, err
	}
	return SaveResult{Profile: prepared.Profile, CredentialMode: credentialMode, Warning: warning}, nil
}

func (service *Service) Delete(ctx context.Context, id ID) error {
	document, err := service.repository.Load(ctx)
	if err != nil {
		return err
	}
	index := -1
	for candidate := range document.Profiles {
		if document.Profiles[candidate].ID == id {
			index = candidate
			break
		}
	}
	if index == -1 {
		return core.NewError("delete profile", core.ErrorValidation, "the selected profile no longer exists", false, nil)
	}
	oldSecret, secretErr := service.secrets.Get(ctx, id)
	oldSecretFound := secretErr == nil
	if secretErr != nil && !errors.Is(secretErr, ErrSecretNotFound) {
		return keychainError("delete profile", "The profile was not deleted because its saved password could not be accessed.", secretErr)
	}
	if err := service.secrets.Delete(ctx, id); err != nil && !errors.Is(err, ErrSecretNotFound) {
		return keychainError("delete profile", "The profile was not deleted because its saved password could not be removed.", err)
	}
	document.Profiles = append(document.Profiles[:index], document.Profiles[index+1:]...)
	if err := service.repository.Save(ctx, document); err != nil {
		service.restoreSecret(context.WithoutCancel(ctx), id, oldSecret, oldSecretFound)
		return err
	}
	service.session.Delete(id)
	return nil
}

func (service *Service) restoreSecret(ctx context.Context, id ID, oldSecret string, found bool) {
	if found {
		_ = service.secrets.Set(ctx, id, oldSecret)
		return
	}
	_ = service.secrets.Delete(ctx, id)
}

func cloneRawMessages(source map[string]json.RawMessage) map[string]json.RawMessage {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]json.RawMessage, len(source))
	for name, value := range source {
		result[name] = append(json.RawMessage(nil), value...)
	}
	return result
}

func keychainError(operation, summary string, cause error) *core.Error {
	return core.NewError(operation, core.ErrorKeychain, summary, true, cause)
}

func keychainWarning(summary string, cause error) *core.Error {
	return core.NewError("save password", core.ErrorKeychain, summary, true, cause)
}
