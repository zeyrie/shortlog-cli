// Package config keeps the CLI's settings in a small JSON file in the user's
// configuration directory. Nothing in it is secret: sessions live in the OS
// credential store, per server.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is the CLI's saved settings.
type Config struct {
	// APIURL is the Shortlog server to connect to. Empty means the default.
	APIURL string `json:"api_url,omitempty"`
}

// Store reads and writes the settings file in a directory.
type Store struct{ dir string }

// Default is the store in the user's configuration directory, such as
// ~/Library/Application Support/shortlog on macOS or ~/.config/shortlog on
// Linux.
func Default() (Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Store{}, err
	}
	return Store{dir: filepath.Join(base, "shortlog")}, nil
}

// In is a store in dir, for tests.
func In(dir string) Store { return Store{dir: dir} }

func (s Store) path() string { return filepath.Join(s.dir, "config.json") }

// Load reads the settings. A missing file is not an error: it means nothing
// has been saved yet.
func (s Store) Load() (Config, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes the settings. It writes a temporary file and renames it into
// place, so a crash never leaves a half-written file behind.
func (s Store) Save(c Config) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path())
}
