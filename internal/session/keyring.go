package session

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// Store keeps one session per API origin in the OS credential manager.
type Store struct{ origin string }

func New(origin string) Store { return Store{origin: origin} }

func (s Store) Load() (string, error) {
	token, err := keyring.Get("shortlog-cli", s.origin)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return token, err
}

func (s Store) Save(token string) error {
	return keyring.Set("shortlog-cli", s.origin, token)
}

func (s Store) Delete() error {
	err := keyring.Delete("shortlog-cli", s.origin)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
