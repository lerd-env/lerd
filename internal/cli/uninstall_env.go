package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/feedback"
)

// sitesWithEnvBackup lists the projects whose env file differs from the copy
// saved before lerd. One that cannot be compared counts, so its copy is named.
func sitesWithEnvBackup(sites []config.Site) []string {
	var paths []string
	for _, s := range sites {
		rel, format := frameworkEnvFile(s.Path)
		envPath := filepath.Join(s.Path, rel)
		changed, err := envfile.ChangedSince(envPath, envBackupPath(envPath), format)
		if changed || (err != nil && !errors.Is(err, os.ErrNotExist)) {
			paths = append(paths, s.Path)
		}
	}
	return paths
}

func registeredEnvBackups() []string {
	reg, err := config.LoadSites()
	if err != nil {
		return nil
	}
	return sitesWithEnvBackup(reg.Sites)
}

// confirmRestoreEnvs returns the projects to restore, none unless asked to: the
// copy predates lerd, so a value changed since would go back too.
func confirmRestoreEnvs() []string {
	paths := registeredEnvBackups()
	if len(paths) == 0 {
		return nil
	}
	for _, p := range paths {
		feedback.Note(p)
	}
	if !feedback.Confirm(fmt.Sprintf("Put back the env values from before lerd in %d project(s)? Keys added since are kept.", len(paths)), false) {
		return nil
	}
	return paths
}

func restoreSiteEnvs(paths []string) {
	for _, path := range paths {
		rel, format := frameworkEnvFile(path)
		if err := restoreEnvBackup(path, rel, format); err != nil {
			feedback.Warn("%v", err)
			continue
		}
		feedback.Note("restored " + filepath.Join(path, rel))
	}
}
