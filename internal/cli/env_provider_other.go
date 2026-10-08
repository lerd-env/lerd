//go:build !darwin

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

func providedEnvSupported() error {
	if config.ProvidedEnvDir() == "" {
		return errors.New("env_provider needs a tmpfs runtime dir (XDG_RUNTIME_DIR)")
	}
	return nil
}

// ensureProvidedEnvDir is a no-op here: the FPM unit's ExecStartPre creates
// the dir, since systemd starts FPM on boot without lerd.
func ensureProvidedEnvDir() {}

// beginProvidedEnvPass is a no-op here: dropping a local file costs no ssh.
func beginProvidedEnvPass() func() { return func() {} }

// RestoreProvidedEnv runs every active site's env_provider. systemd starts FPM
// and the workers on boot without lerd start, so the watcher calls this on its
// own start to refill the files a reboot emptied from tmpfs.
func RestoreProvidedEnv() {
	reg, err := config.LoadSites()
	if err != nil {
		feedback.Warn("env_provider restore: %v", err)
		return
	}
	for _, s := range reg.Sites {
		if s.Paused || s.Ignored {
			continue
		}
		if err := refreshProvidedEnv(s, false); err != nil {
			feedback.Warn("%s: %v", s.Name, err)
		}
		// An unlink during the provider run dropped the file before we wrote
		// it; teardown updates the registry first, so this catches it. A
		// registry that cannot be read proves nothing, so the file stays.
		cur, err := config.LoadSites()
		if err == nil && !slices.ContainsFunc(cur.Sites, func(c config.Site) bool {
			return c.Name == s.Name && !c.Ignored
		}) {
			dropProvidedEnv(s.Name)
		}
	}
}

func storeProvidedEnv(siteName string, data []byte) error {
	return writeProvidedEnv(config.ProvidedEnvFile(siteName), data)
}

func dropProvidedEnv(siteName string) {
	if f := config.ProvidedEnvFile(siteName); f != "" {
		_ = os.Remove(f)
	}
}

// writeProvidedEnv replaces the file atomically with owner-only permissions, so
// PHP never reads a half-written file and no other user can read it.
func writeProvidedEnv(file string, data []byte) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".provided-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}
