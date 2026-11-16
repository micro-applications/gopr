package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
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

func TestMigrateDirCopiesOnce(t *testing.T) {
	dir := t.TempDir()
	oldDir, newDir := filepath.Join(dir, "Godiff", "analyses"), filepath.Join(dir, "GoPR", "analyses")
	writeFile(t, oldDir, "a.json", "a")
	writeFile(t, oldDir, "b.json", "b")
	if err := migrateDir(newDir, oldDir); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a.json": "a", "b.json": "b"} {
		if got, _ := os.ReadFile(filepath.Join(newDir, name)); string(got) != want {
			t.Errorf("%s: got %q", name, got)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(newDir)); len(entries) != 1 {
		t.Errorf("left beside it: %v", entries)
	}
	// Once there, it is left alone.
	writeFile(t, oldDir, "a.json", "changed")
	if err := migrateDir(newDir, oldDir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(newDir, "a.json")); string(got) != "a" {
		t.Errorf("the second call overwrote: %q", got)
	}
	// Without an old directory, nothing is made.
	none := filepath.Join(dir, "none")
	if err := migrateDir(none, filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(none); err == nil {
		t.Error("the new directory was made")
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

func TestStoreSaveReplacesFile(t *testing.T) {
	dir := t.TempDir()
	s := &store{path: filepath.Join(dir, "state.json")}
	os.WriteFile(s.path, []byte("{}"), 0o644)
	// A link to the file keeps what it held: the save writes a new file in
	// its place rather than into it, which a reader could see half written.
	link := filepath.Join(dir, "link")
	if err := os.Link(s.path, link); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.addRecent(fmt.Sprintf("/repo%d", i))
		}()
	}
	wg.Wait()
	if got, _ := os.ReadFile(link); string(got) != "{}" {
		t.Errorf("the save wrote into the file: %q", got)
	}
	var saved storeData
	data, _ := os.ReadFile(s.path)
	if err := json.Unmarshal(data, &saved); err != nil || len(saved.Recent) != 8 {
		t.Errorf("saved %q: %v", data, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("files left: %v", entries)
	}
}

func TestUIFontSize(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, 13}, {5, 10}, {15, 15}, {40, 24}} {
		s := Settings{UIFontSize: tc.in}
		s.normalize()
		if s.UIFontSize != tc.want {
			t.Errorf("uiFontSize %d normalized to %d, want %d", tc.in, s.UIFontSize, tc.want)
		}
	}
}
