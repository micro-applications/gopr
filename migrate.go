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
