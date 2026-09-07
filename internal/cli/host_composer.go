package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// hostComposerOnPath returns the composer the user installed themselves, or ""
// when there is none behind lerd's. exec.LookPath is no use here: lerd's bin dir
// goes first on PATH, so it answers with the shim rather than with the binary
// that shim now fronts.
func hostComposerOnPath() string {
	binDir := config.BinDir()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || dir == binDir {
			continue
		}
		candidate := filepath.Join(dir, "composer")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0111 != 0 {
			return candidate
		}
	}
	return ""
}

// noteShadowedComposer says out loud that the PATH shim now sits in front of a
// composer the user already had. lerd runs its own pinned phar inside the
// container, so nothing about the host copy changes, but a silent takeover
// reads as lerd having installed a second composer for no reason.
func noteShadowedComposer() {
	if pathShimDisabled() {
		return
	}
	if host := hostComposerOnPath(); host != "" {
		feedback.Note(fmt.Sprintf("your composer at %s is untouched; lerd's own comes first on PATH and runs inside the project's container (lerd path:disable puts yours back in front)", host))
	}
}
