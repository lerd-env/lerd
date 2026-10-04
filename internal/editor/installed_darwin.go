package editor

import (
	"os"
	"path/filepath"
)

// installedOffPath finds an editor not on PATH by its app bundle.
func (e Editor) installedOffPath() bool {
	home, _ := os.UserHomeDir()
	for _, app := range e.apps {
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			if _, err := os.Stat(filepath.Join(dir, app)); err == nil {
				return true
			}
		}
	}
	return false
}
