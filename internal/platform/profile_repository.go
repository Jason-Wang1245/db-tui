package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jason-Wang1245/db-tui/internal/core"
	"github.com/Jason-Wang1245/db-tui/internal/profile"
)

const maximumProfilesFileSize = 8 << 20

var _ profile.Repository = JSONProfileRepository{}

type JSONProfileRepository struct {
	paths profile.ConfigPaths
}

func NewJSONProfileRepository(paths profile.ConfigPaths) JSONProfileRepository {
	return JSONProfileRepository{paths: paths}
}

func (repository JSONProfileRepository) Load(ctx context.Context) (profile.Document, error) {
	if err := ctx.Err(); err != nil {
		return profile.Document{}, err
	}
	exists, err := secureConfigDirectory(repository.paths.ConfigDirectory(), false)
	if err != nil {
		return profile.Document{}, err
	}
	if !exists {
		return profile.Document{Version: profile.CurrentDocumentVersion, Profiles: []profile.Profile{}}, nil
	}
	path := repository.paths.ProfilesFile()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return profile.Document{Version: profile.CurrentDocumentVersion, Profiles: []profile.Profile{}}, nil
	}
	if err != nil {
		return profile.Document{}, persistenceError("load profiles", "could not inspect the local profiles file", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return profile.Document{}, persistenceError("load profiles", "the local profiles file must not be a symbolic link", nil)
	}
	if info.Size() > maximumProfilesFileSize {
		return profile.Document{}, persistenceError("load profiles", "the local profiles file is unexpectedly large", nil)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return profile.Document{}, persistenceError("load profiles", "could not secure the local profiles file", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return profile.Document{}, persistenceError("load profiles", "could not open the local profiles file", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximumProfilesFileSize+1))
	if err != nil {
		return profile.Document{}, persistenceError("load profiles", "could not read the local profiles file", err)
	}
	if len(data) > maximumProfilesFileSize {
		return profile.Document{}, persistenceError("load profiles", "the local profiles file is unexpectedly large", nil)
	}
	var document profile.Document
	if err := json.Unmarshal(data, &document); err != nil {
		return profile.Document{}, persistenceError("load profiles", "the local profiles file is invalid JSON", err)
	}
	if document.Version != profile.CurrentDocumentVersion {
		return profile.Document{}, core.NewError(
			"load profiles",
			core.ErrorUnsupported,
			fmt.Sprintf("profile format version %d is not supported by this build", document.Version),
			false,
			nil,
		)
	}
	if document.Profiles == nil {
		document.Profiles = []profile.Profile{}
	}
	if err := validateProfileDocument(document); err != nil {
		return profile.Document{}, err
	}
	return document, nil
}

func (repository JSONProfileRepository) Save(ctx context.Context, document profile.Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if document.Version == 0 {
		document.Version = profile.CurrentDocumentVersion
	}
	if document.Version != profile.CurrentDocumentVersion {
		return core.NewError("save profiles", core.ErrorUnsupported, "cannot write an unsupported profile format version", false, nil)
	}
	if err := validateProfileDocument(document); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return persistenceError("save profiles", "could not encode local profiles", err)
	}
	data = append(data, '\n')
	directory := repository.paths.ConfigDirectory()
	if _, err := secureConfigDirectory(directory, true); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".profiles-*.json")
	if err != nil {
		return persistenceError("save profiles", "could not create a temporary profiles file", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		_ = temporary.Close()
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return persistenceError("save profiles", "could not secure the temporary profiles file", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return persistenceError("save profiles", "could not write local profiles", err)
	}
	if err := temporary.Sync(); err != nil {
		return persistenceError("save profiles", "could not flush local profiles", err)
	}
	if err := temporary.Close(); err != nil {
		return persistenceError("save profiles", "could not close the temporary profiles file", err)
	}
	if err := os.Rename(temporaryPath, repository.paths.ProfilesFile()); err != nil {
		return persistenceError("save profiles", "could not replace the local profiles file", err)
	}
	keepTemporary = false
	if err := os.Chmod(repository.paths.ProfilesFile(), 0o600); err != nil {
		return persistenceError("save profiles", "could not secure the local profiles file", err)
	}
	if directoryHandle, err := os.Open(filepath.Clean(directory)); err == nil {
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

func secureConfigDirectory(directory string, create bool) (bool, error) {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return false, nil
		}
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return false, persistenceError("save profiles", "could not create the local configuration directory", err)
		}
		info, err = os.Lstat(directory)
	}
	if err != nil {
		return false, persistenceError("load profiles", "could not inspect the local configuration directory", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, persistenceError("load profiles", "the local configuration path must be a directory and not a symbolic link", nil)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return false, persistenceError("load profiles", "could not secure the local configuration directory", err)
	}
	return true, nil
}

func validateProfileDocument(document profile.Document) error {
	if hasSensitiveUnknownField(document.Unknown) {
		return persistenceError("load profiles", "the local profiles file contains a forbidden secret field", nil)
	}
	identifiers := make(map[profile.ID]bool, len(document.Profiles))
	for _, saved := range document.Profiles {
		if saved.ID == "" || identifiers[saved.ID] {
			return persistenceError("load profiles", "the local profiles file contains a missing or duplicate profile identifier", nil)
		}
		identifiers[saved.ID] = true
		if hasSensitiveUnknownField(saved.Unknown) {
			return persistenceError("load profiles", "the local profiles file contains a forbidden secret field", nil)
		}
		if _, err := profile.ValidateDraft(profile.DraftFromProfile(saved, false), document); err != nil {
			return persistenceError("load profiles", "the local profiles file contains an invalid profile", err)
		}
	}
	return nil
}

func hasSensitiveUnknownField(fields map[string]json.RawMessage) bool {
	for name := range fields {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "password", "passfile", "connection_url", "original_url", "secret":
			return true
		}
	}
	return false
}

func persistenceError(operation, summary string, cause error) *core.Error {
	return core.NewError(operation, core.ErrorPersistence, summary, true, cause)
}
