package sitedoctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// guardedHomeFolders are the folders macOS puts behind a Files and Folders
// prompt. Pictures and Movies are guarded too but no project lives there.
var guardedHomeFolders = []string{"Documents", "Desktop", "Downloads"}

// protectedPathCheck warns when a macOS project sits in a folder the system
// guards and lerd is still driving fnm. Tooling run for the site reads the
// project, macOS attributes that to fnm rather than to lerd, and the grant it
// remembers is keyed to that binary's signing identity. fnm ships unsigned, so
// the next pinned version is a stranger to the system and the same folder is
// asked for again (#1906). mise is signed under a Developer ID that survives an
// update, which is the way out the warning names. Split from the caller and
// given its platform so it is testable off a Mac.
func protectedPathCheck(goos, path, home, manager string) (Check, bool) {
	if goos != "darwin" || manager != "fnm" || home == "" {
		return Check{}, false
	}
	// The grant follows the real files, so a project reached through a symlink
	// from somewhere unguarded is guarded all the same. Home is resolved too, or
	// a home behind a symlink never matches the path we just canonicalised.
	real := resolve(path)
	home = resolve(home)
	for _, folder := range guardedHomeFolders {
		guarded := filepath.Join(home, folder)
		if real != guarded && !strings.HasPrefix(real, guarded+string(filepath.Separator)) {
			continue
		}
		return Check{
			Name:   "macos_protected_path",
			Label:  "macOS folder access",
			Status: StatusWarn,
			Detail: fmt.Sprintf("This project is in %s, which macOS guards, so fnm asks for access to it and asks again after every fnm version bump, losing the answer you gave. Run 'lerd node:manager mise' to switch to a signed manager the grant survives, or keep the project outside %s.",
				folder, strings.Join(guardedHomeFolders, ", ")),
		}, true
	}
	return Check{}, false
}

// checkProtectedPath is the live wiring for protectedPathCheck: this host, this
// user's home, and whichever Node manager lerd is driving.
func checkProtectedPath(path string) (Check, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Check{}, false
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		return Check{}, false
	}
	return protectedPathCheck(runtime.GOOS, path, home, cfg.NodeManager())
}

// resolve canonicalises a path, leaving it untouched when it cannot be read.
func resolve(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
