package platform

import (
	"context"
	"errors"

	"github.com/zalando/go-keyring"

	"github.com/Jason-Wang1245/db-tui/internal/profile"
)

var ErrSecretNotFound = profile.ErrSecretNotFound

var _ profile.SecretStore = Keyring{}

type Keyring struct {
	service string
}

func NewKeyring(service string) Keyring {
	return Keyring{service: service}
}

func (k Keyring) Get(ctx context.Context, id profile.ID) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	secret, err := keyring.Get(k.service, string(id))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrSecretNotFound
	}
	return secret, err
}

func (k Keyring) Set(ctx context.Context, id profile.ID, secret string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return keyring.Set(k.service, string(id), secret)
}

func (k Keyring) Delete(ctx context.Context, id profile.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := keyring.Delete(k.service, string(id))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
