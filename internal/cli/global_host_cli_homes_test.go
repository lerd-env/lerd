package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// seedGlobalPackage declares a binary as host-only and registers the package in
// the composer home given, which is what the routing decision reads.
func seedGlobalPackage(t *testing.T, composerHome, bin string) {
	t.Helper()
	store := filepath.Join(os.Getenv("XDG_DATA_HOME"), "lerd", "frameworks")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "index.json"),
		[]byte(`{"frameworks":[],"packages":[{"name":"acme/cloud-cli"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.StorePackagesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.StorePackagesDir(), "acme-cloud-cli.yaml"),
		[]byte("package: acme/cloud-cli\nhost_binaries:\n  - "+bin+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(composerHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composerHome, "composer.json"),
		[]byte(`{"require": {"acme/cloud-cli": "^1.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// composer only takes the XDG location when an XDG_ variable or /etc/xdg says
// so, and otherwise uses ~/.composer. A machine can have global packages in
// either, and a tool in the one lerd did not pick was left running in the
// container with its browser callback unreachable, which is the whole point of
// moving it to the host.
func TestRunGlobalHostCLI_findsABinaryInTheLegacyComposerHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COMPOSER_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	legacy := filepath.Join(home, ".composer")
	seedGlobalPackage(t, legacy, "cloud")

	bin := filepath.Join(legacy, "vendor", "bin", "cloud")
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{bin, "login"}, nil); !took {
		t.Error("a global binary under ~/.composer must still be taken out of the container")
	}
}

// The XDG home keeps working, so a machine that uses that one is unaffected.
func TestRunGlobalHostCLI_findsABinaryInTheXDGComposerHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COMPOSER_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	xdg := filepath.Join(home, ".config", "composer")
	seedGlobalPackage(t, xdg, "cloud")

	bin := filepath.Join(xdg, "vendor", "bin", "cloud")
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{bin, "login"}, nil); !took {
		t.Error("a global binary under the XDG home must be taken out of the container")
	}
}

// COMPOSER_HOME is explicit, so nothing else should be consulted.
func TestRunGlobalHostCLI_honoursComposerHomeExactly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	explicit := filepath.Join(home, "elsewhere")
	t.Setenv("COMPOSER_HOME", explicit)
	seedGlobalPackage(t, explicit, "cloud")

	// The same name under a home COMPOSER_HOME did not name is not it.
	other := filepath.Join(home, ".composer", "vendor", "bin", "cloud")
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{other, "login"}, nil); took {
		t.Error("COMPOSER_HOME is explicit; another home must not be consulted")
	}
	if _, took, _ := runGlobalHostCLI(t.TempDir(), []string{filepath.Join(explicit, "vendor", "bin", "cloud")}, nil); !took {
		t.Error("the binary under COMPOSER_HOME must be taken")
	}
}
