// Package profile owns saved profile types, validation, and persistence
// coordination contracts.
package profile

import (
	"context"
	"time"
)

type ID string

type Profile struct {
	ID                 ID
	Name               string
	Host               string
	Port               uint16
	Database           string
	User               string
	SSLMode            string
	AdvancedParameters map[string]string
	SavePassword       bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastUsedAt         time.Time
}

type Document struct {
	Version  int
	Profiles []Profile
}

type Repository interface {
	Load(context.Context) (Document, error)
	Save(context.Context, Document) error
}

type SecretStore interface {
	Get(context.Context, ID) (string, error)
	Set(context.Context, ID, string) error
	Delete(context.Context, ID) error
}

type ConfigPaths interface {
	ConfigDirectory() string
	ProfilesFile() string
}
