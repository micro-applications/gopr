package main

import (
	"errors"
	"os"
	"path/filepath"
)

// migrateFile copies oldPath to newPath, creating its directories, when
// newPath does not exist and oldPath does. Otherwise it does nothing.
func migrateFile(newPath, oldPath string) error {
	if _, err := os.Stat(newPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(newPath, data, 0o644)
}

// migrateDir copies the files of oldDir into newDir when newDir does not
// exist and oldDir does. It copies into a directory beside newDir, renamed
// to it once whole, so that a copy cut short is made again next time.
func migrateDir(newDir, oldDir string) error {
	if _, err := os.Stat(newDir); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(newDir), "."+filepath.Base(newDir)+".*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(oldDir, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(tmp, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, newDir)
}
