package podman

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// lerd_devtools C source, compiled into every FPM image by the Containerfile's
// builder stage. writeDevtoolsSource stages it into the local build context so
// the `COPY internal/podman/devtools` resolves the same way it does in CI, and
// devtoolsSourceHash feeds the Containerfile's sync marker.
//
//go:embed devtools
var devtoolsSrcFS embed.FS

//go:embed devtoolsbridge
var devtoolsBridgeFS embed.FS

// DevtoolsIni returns the conf.d ini with runtime placeholders substituted.
// {{ DEVTOOLS_TARGET }} resolves through config.DevtoolsBridgeTarget (the same
// socket the debug bridge ships to). {{ DEVTOOLS_KINDS }} is the comma-separated
// set of event kinds the extension should capture.
func DevtoolsIni() (string, error) {
	b, err := devtoolsBridgeFS.ReadFile("devtoolsbridge/96-lerd-devtools.ini")
	if err != nil {
		return "", fmt.Errorf("devtools ini embed: %w", err)
	}
	out := strings.ReplaceAll(string(b), "{{ DEVTOOLS_TARGET }}", config.DevtoolsBridgeTarget())
	out = strings.ReplaceAll(out, "{{ DEVTOOLS_KINDS }}", "query")
	return out, nil
}

// EnsureDevtoolsAssets writes the devtools conf.d ini to its host path so the
// always-mounted FPM volume has a regular file (not a podman-auto-created dir)
// at the bind-mount source. Idempotent; runs regardless of Devtools.Enabled
// because the FPM quadlet always mounts the ini — the sentinel controls
// capture, exactly like the debug bridge.
func EnsureDevtoolsAssets() error {
	dir := config.DevtoolsAssetsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating devtools dir: %w", err)
	}
	ini, err := DevtoolsIni()
	if err != nil {
		return err
	}
	path := config.DevtoolsIniFile()
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			if rmErr := os.RemoveAll(path); rmErr != nil {
				return fmt.Errorf("removing stale devtools ini directory %s: %w", path, rmErr)
			}
		} else if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == ini {
			return nil
		}
	}
	if err := os.WriteFile(path, []byte(ini), 0644); err != nil {
		return fmt.Errorf("writing devtools ini %s: %w", path, err)
	}
	return nil
}

// SetDevtoolsWorkersFlag flips the sentinel that opts queue/scheduler worker
// queries into capture. Present = workers captured, absent = skipped. Like the
// enable flag, it sits under the dumps assets dir (mounted at
// /usr/local/etc/lerd) so no FPM restart is needed.
func SetDevtoolsWorkersFlag(enabled bool) error {
	flag := config.DevtoolsWorkersFlagFile()
	if enabled {
		if err := os.MkdirAll(filepath.Dir(flag), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(flag, []byte("1\n"), 0644); err != nil {
			return fmt.Errorf("writing devtools workers flag: %w", err)
		}
		return nil
	}
	if err := os.Remove(flag); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing devtools workers flag: %w", err)
	}
	return nil
}

// writeDevtoolsSource copies the embedded extension source into the build
// context at internal/podman/devtools so the Containerfile's
// `COPY internal/podman/devtools` resolves the same way it does in CI (where
// the context is the repo root). Only the slow local build needs this; the
// prebuilt base already carries the compiled extension.
func writeDevtoolsSource(ctxDir string) error {
	dst := filepath.Join(ctxDir, "internal", "podman", "devtools")
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := devtoolsSrcFS.ReadDir("devtools")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := devtoolsSrcFS.ReadFile("devtools/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0644); err != nil {
			return err
		}
	}
	return nil
}

// devtoolsSourceHash is the short sha256 of the extension source (every file
// under devtools/, in name order). The Containerfile carries this value in its
// `lerd_devtools-src-sha256:` marker so that any change to the C source drifts
// the Containerfile hash, which both rebuilds the prebuilt base in CI and trips
// NeedsFPMRebuild for updating users. TestDevtoolsSourceMarkerInSync asserts the
// marker matches this, so the marker can't silently fall out of date.
func devtoolsSourceHash() (string, error) {
	entries, err := devtoolsSrcFS.ReadDir("devtools")
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		b, err := devtoolsSrcFS.ReadFile("devtools/" + n)
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:12], nil
}
