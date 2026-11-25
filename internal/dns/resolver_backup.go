package dns

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

// backupForeignResolver copies a resolver file another tool wrote to backup
// before lerd replaces it, and reports whether the copy is new. A missing file
// drops the old copy: its owner is gone, so there is nothing to give back.
func backupForeignResolver(file, backup string, own []byte) (bool, error) {
	body, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if bytes.Equal(bytes.TrimSpace(body), bytes.TrimSpace(own)) || isFileContent(backup, body) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(backup), 0755); err != nil {
		return false, err
	}
	return true, os.WriteFile(backup, body, 0644)
}

// restoreResolvers writes back the saved copy of each lerd resolver file and
// returns the ones that have none, or failed, for the caller to remove.
func restoreResolvers(files []string, backupDir string, write func(string, []byte) error) []string {
	var left []string
	for _, f := range files {
		backup := filepath.Join(backupDir, filepath.Base(f))
		body, err := os.ReadFile(backup)
		if err != nil || write(f, body) != nil {
			left = append(left, f)
			continue
		}
		os.Remove(backup) //nolint:errcheck
	}
	return left
}
