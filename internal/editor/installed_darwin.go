package editor

import (
	"os"
	"path/filepath"
)

// installedOffPath finds an editor not on PATH by its app bundle.
func (e Editor) installedOffPath() bool {
	_, ok := e.appBundle()
	return ok
}

// dirCommandOffPath opens a folder in the editor's app bundle, since an app
// installed without its shell command has nothing on PATH to run.
func (e Editor) dirCommandOffPath(dir string) []string {
	if app, ok := e.appBundle(); ok {
		return []string{"open", "-a", app, dir}
	}
	return nil
}

// appBundleDirs are where app bundles live; a variable so a test can keep the
// machine's own out.
var appBundleDirs = func() []string {
	home, _ := os.UserHomeDir()
	return []string{"/Applications", filepath.Join(home, "Applications")}
}

func (e Editor) appBundle() (string, bool) {
	for _, app := range e.apps {
		for _, dir := range appBundleDirs() {
			p := filepath.Join(dir, app)
			if _, err := os.Stat(p); err == nil {
				return p, true
			}
		}
	}
	return "", false
}
