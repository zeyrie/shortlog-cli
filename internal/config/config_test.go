package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFileIsEmpty(t *testing.T) {
	c, err := In(t.TempDir()).Load()
	if err != nil || c.APIURL != "" {
		t.Fatalf("missing file: %+v, %v", c, err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "shortlog")
	s := In(dir)
	if err := s.Save(Config{APIURL: "https://notes.example"}); err != nil {
		t.Fatal(err)
	}
	c, err := s.Load()
	if err != nil || c.APIURL != "https://notes.example" {
		t.Fatalf("round trip: %+v, %v", c, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}
}

func TestCorruptFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := In(dir).Load(); err == nil {
		t.Fatal("a corrupt file should be reported, not ignored")
	}
}
