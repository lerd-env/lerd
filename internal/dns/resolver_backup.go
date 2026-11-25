package dns

import (
	"bytes"
	"errors"
	"fmt"
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

// restoreResolvers writes back the saved copy of each lerd resolver file. It
// returns the files that have none, for the caller to remove, and an error for
// those whose copy could not be put back, which stay as they are.
func restoreResolvers(files []string, backupDir string, write func(string, []byte) error) ([]string, error) {
	var unsaved []string
	var errs []error
	for _, f := range files {
		backup := filepath.Join(backupDir, filepath.Base(f))
		body, err := os.ReadFile(backup)
		if errors.Is(err, os.ErrNotExist) {
			unsaved = append(unsaved, f)
			continue
		}
		if err == nil {
			err = write(f, body)
		}
		if err == nil {
			err = os.Remove(backup)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("could not put %s back from %s: %w", f, backup, err))
		}
	}
	return unsaved, errors.Join(errs...)
}
