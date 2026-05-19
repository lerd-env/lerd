package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// shimSync describes one category of lerd-managed wrappers in targetBin.
// Each category uses its own marker so a sync over one source dir never
// touches wrappers belonging to another category.
type shimSync struct {
	sourceBin string                      // directory whose executables to mirror
	targetBin string                      // directory where wrapper scripts live
	marker    string                      // unique substring identifying our wrappers
	bodyFor   func(realBin string) string // wrapper script content for a given source path
}

// run mirrors every regular file in sourceBin into targetBin as a small
// shell wrapper, preserving any file in targetBin that does not carry the
// marker (so user-installed binaries or other lerd shims are left alone)
// and removing orphan wrappers whose source has disappeared.
func (s shimSync) run() error {
	if err := os.MkdirAll(s.targetBin, 0o755); err != nil {
		return err
	}

	want := map[string]bool{}
	entries, err := os.ReadDir(s.sourceBin)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		want[name] = true
		wrapperPath := filepath.Join(s.targetBin, name)
		if existing, ok := readShimHead(wrapperPath); ok && !strings.Contains(existing, s.marker) {
			continue
		} else if !ok {
			if _, statErr := os.Stat(wrapperPath); statErr == nil {
				continue
			}
		}
		body := s.bodyFor(filepath.Join(s.sourceBin, name))
		if err := os.WriteFile(wrapperPath, []byte(body), 0o755); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(s.targetBin)
	if err != nil {
		return err
	}
	for _, e := range targetEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if want[name] {
			continue
		}
		full := filepath.Join(s.targetBin, name)
		head, ok := readShimHead(full)
		if !ok {
			continue
		}
		if !strings.Contains(head, s.marker) {
			continue
		}
		_ = os.Remove(full)
	}
	return nil
}

// readShimHead returns the first ~256 bytes of path as a string, but only if
// the file starts with a shell shebang. The second return is false when the
// file isn't a shell script, so callers can skip native binaries without
// risking a false-positive marker match on string constants inside them.
func readShimHead(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	if n < 2 || buf[0] != '#' || buf[1] != '!' {
		return "", false
	}
	return string(buf[:n]), true
}

// nodeShimMarker tags wrapper scripts lerd writes for npm globals.
const nodeShimMarker = "lerd-managed npm global shim"

// composerShimMarker tags wrapper scripts lerd writes for composer globals.
const composerShimMarker = "lerd-managed composer global shim"

// syncNodeGlobalBins mirrors sourceBin into targetBin via `fnm exec`, so
// `#!/usr/bin/env node` shebangs resolve against the fnm-managed default
// node from any directory.
func syncNodeGlobalBins(sourceBin, targetBin, fnmPath string) error {
	return shimSync{
		sourceBin: sourceBin,
		targetBin: targetBin,
		marker:    nodeShimMarker,
		bodyFor: func(realBin string) string {
			// --using=default so the wrapper works from any directory; without
			// it fnm errors out unless the cwd has a .nvmrc/.node-version.
			return fmt.Sprintf("#!/bin/sh\n# %s\nexec %q exec --using=default -- %q \"$@\"\n", nodeShimMarker, fnmPath, realBin)
		},
	}.run()
}

// syncComposerGlobalBins mirrors sourceBin into targetBin via `lerd php`, so
// `#!/usr/bin/env php` shebangs on composer globals (psysh, phpunit, carbon,
// laravel/installer, etc.) resolve against lerd's container-backed PHP.
func syncComposerGlobalBins(sourceBin, targetBin, lerdPath string) error {
	return shimSync{
		sourceBin: sourceBin,
		targetBin: targetBin,
		marker:    composerShimMarker,
		bodyFor: func(realBin string) string {
			return fmt.Sprintf("#!/bin/sh\n# %s\nexec %q php %q \"$@\"\n", composerShimMarker, lerdPath, realBin)
		},
	}.run()
}
