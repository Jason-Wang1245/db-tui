// Package profile owns saved profile types, validation, and persistence
// coordination contracts.
package profile

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type ID string

type Profile struct {
	ID                 ID                         `json:"id"`
	Name               string                     `json:"name"`
	Host               string                     `json:"host"`
	Port               uint16                     `json:"port"`
	Database           string                     `json:"database"`
	User               string                     `json:"user"`
	SSLMode            string                     `json:"ssl_mode"`
	AdvancedParameters map[string]string          `json:"advanced_parameters,omitempty"`
	SavePassword       bool                       `json:"save_password"`
	CreatedAt          time.Time                  `json:"created_at"`
	UpdatedAt          time.Time                  `json:"updated_at"`
	LastUsedAt         time.Time                  `json:"last_used_at,omitzero"`
	Unknown            map[string]json.RawMessage `json:"-"`
}

type Document struct {
	Version  int                        `json:"version"`
	Profiles []Profile                  `json:"profiles"`
	Unknown  map[string]json.RawMessage `json:"-"`
}

const CurrentDocumentVersion = 1

var ErrSecretNotFound = errors.New("secret not found")

type Parameter struct {
	Name  string
	Value string
}

type Draft struct {
	ID                 ID
	Name               string
	Host               string
	Port               string
	Database           string
	User               string
	Password           string
	ReplacePassword    bool
	HasStoredPassword  bool
	SavePassword       bool
	SSLMode            string
	AdvancedParameters []Parameter
}

func NewDraft() Draft {
	return Draft{Port: "5432", SSLMode: "prefer", SavePassword: true}
}

func DraftFromProfile(saved Profile, hasStoredPassword bool) Draft {
	return Draft{
		ID:                 saved.ID,
		Name:               saved.Name,
		Host:               saved.Host,
		Port:               formatPort(saved.Port),
		Database:           saved.Database,
		User:               saved.User,
		HasStoredPassword:  hasStoredPassword,
		SavePassword:       saved.SavePassword,
		SSLMode:            saved.SSLMode,
		AdvancedParameters: sortedParameters(saved.AdvancedParameters),
	}
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

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID() (ID, error)
}
