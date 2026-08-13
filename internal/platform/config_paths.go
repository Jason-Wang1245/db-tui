// Package platform contains concrete operating-system adapters. These adapters
// are invoked from commands and do not depend on Bubble Tea.
package platform

import (
	"path/filepath"

	"github.com/Jason-Wang1245/db-tui/internal/profile"
)

var _ profile.ConfigPaths = ConfigPaths{}

type ConfigPaths struct {
	baseDirectory string
}

func NewConfigPaths(userConfigDirectory string) ConfigPaths {
	return ConfigPaths{baseDirectory: filepath.Join(userConfigDirectory, "db-tui")}
}

func (p ConfigPaths) ConfigDirectory() string {
	return p.baseDirectory
}

func (p ConfigPaths) ProfilesFile() string {
	return filepath.Join(p.baseDirectory, "profiles.json")
}
