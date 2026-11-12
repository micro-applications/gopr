package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/godiff/internal/github"
)

func TestRecent(t *testing.T) {
	s := &store{}
	for i := range maxRecent + 2 {
		s.addRecent(fmt.Sprintf("/repo%d", i))
	}
	got := s.recent()
	if len(got) != maxRecent || got[0] != fmt.Sprintf("/repo%d", maxRecent+1) {
		t.Fatalf("got %v", got)
	}
	// Opening one again brings it first, once.
	if !s.addRecent("/repo5") {
		t.Error("moving a repository first changed nothing")
	}
	if s.addRecent("/repo5") {
		t.Error("the first repository changed the list")
	}
	got = s.recent()
	if got[0] != "/repo5" || len(got) != maxRecent || slices.Index(got[1:], "/repo5") >= 0 {
		t.Errorf("got %v", got)
	}
}

func TestMigrateFileCopiesOnce(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "old"), filepath.Join(dir, "sub", "new")
	os.WriteFile(oldPath, []byte("a"), 0o644)
	if err := migrateFile(newPath, oldPath); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "a" {
		t.Fatalf("got %q", got)
	}
	os.WriteFile(oldPath, []byte("b"), 0o644)
	migrateFile(newPath, oldPath)
	if got, _ := os.ReadFile(newPath); string(got) != "a" {
		t.Errorf("the second call overwrote: %q", got)
	}
}

func TestMigrateFileNoOld(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "new")
	if err := migrateFile(newPath, filepath.Join(dir, "old")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newPath); err == nil {
		t.Error("the new file was created")
	}
}

func TestWindowTitle(t *testing.T) {
	got := windowTitle("/x/app", pullSource(github.Repo{Owner: "acme", Name: "app"}, 7))
	if got != "app/#7 · GoPR" {
		t.Errorf("got %q", got)
	}
}

func TestNewConfigDoesNotMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := filepath.Join(home, ".godiff", "godiff.jsonc")
	os.MkdirAll(filepath.Dir(old), 0o755)
	os.WriteFile(old, []byte("{}"), 0o644)
	c := newConfig()
	if _, err := os.Stat(c.path); err == nil {
		t.Error("newConfig migrated the old settings")
	}
	c.migrateConfig()
	if _, err := os.Stat(c.path); err != nil {
		t.Errorf("migrateConfig did not copy: %v", err)
	}
}
