package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// stageNativeBuild fakes what the release tarball unpacks to.
func stageNativeBuild(t *testing.T, version string) string {
	t.Helper()
	stage := t.TempDir()
	for _, name := range []string{"php-native-" + version, "php-native-fpm-" + version} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte("binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mods := filepath.Join(stage, "modules")
	if err := os.MkdirAll(mods, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xdebug.so", "pcov.so", "lerd_devtools-" + version + ".so"} {
		if err := os.WriteFile(filepath.Join(mods, name), []byte("so"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return stage
}

func TestUnpackNativePHPPlacesBinariesAndModules(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := unpackNativePHP(stageNativeBuild(t, "8.4"), "8.4", "8.4.24"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{nativephp.BinaryPath("8.4"), nativephp.FPMBinaryPath("8.4")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s is not executable (%v)", p, info.Mode().Perm())
		}
	}
	if got := nativephp.DevtoolsExtensionPath("8.4"); got == "" {
		t.Error("the query-capture collector was not installed")
	}
	if _, err := os.Stat(filepath.Join(nativephp.ModulesDir("8.4"), "xdebug.so")); err != nil {
		t.Errorf("xdebug.so was not installed: %v", err)
	}
	// The stamp is what tells an update whether the newest patch is already on
	// disk, so an install that does not record one asks to be redownloaded.
	if got := tools.InstalledVersion(nativeTool("8.4")); got != "8.4.24" {
		t.Errorf("stamp = %q, want 8.4.24", got)
	}
}

// A patch bump replaces the previous build, and must not leave a module from
// it behind: the old .so does not load into the new binary.
func TestUnpackNativePHPReplacesAnEarlierPatch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := unpackNativePHP(stageNativeBuild(t, "8.4"), "8.4", "8.4.23"); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(nativephp.ModulesDir("8.4"), "leftover.so")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unpackNativePHP(stageNativeBuild(t, "8.4"), "8.4", "8.4.24"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a module from the previous patch survived the update")
	}
	if got := tools.InstalledVersion(nativeTool("8.4")); got != "8.4.24" {
		t.Errorf("stamp = %q, want 8.4.24", got)
	}
}

// Installing one minor must not disturb another: they are separate builds that
// happen to ship modules under the same file names.
func TestUnpackNativePHPLeavesOtherVersionsAlone(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := unpackNativePHP(stageNativeBuild(t, "8.4"), "8.4", "8.4.24"); err != nil {
		t.Fatal(err)
	}
	if err := unpackNativePHP(stageNativeBuild(t, "8.5"), "8.5", "8.5.3"); err != nil {
		t.Fatal(err)
	}
	if nativephp.DevtoolsExtensionPath("8.4") == "" {
		t.Error("installing 8.5 removed 8.4's collector")
	}
	if got := tools.InstalledVersion(nativeTool("8.4")); got != "8.4.24" {
		t.Errorf("8.4 stamp = %q after installing 8.5, want 8.4.24", got)
	}
	_ = config.BinDir()
}

func TestNativeUpdatePlan(t *testing.T) {
	cases := []struct {
		name              string
		pinned, installed string
		want              string
		wantUpdate        bool
	}{
		{"a newer patch is published", "8.4.24", "8.4.23", "8.4.24", true},
		{"already on the pinned patch", "8.4.24", "8.4.24", "8.4.24", false},
		{"nothing installed yet", "8.4.24", "", "8.4.24", true},
		// The pin is the only statement of what lerd publishes, so a machine
		// ahead of it is brought back to it rather than left on a build that is
		// no longer offered.
		{"installed ahead of the pin", "8.4.23", "8.4.24", "8.4.23", true},
		{"no pin at all", "", "8.4.24", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, update := nativeUpdatePlan(tc.pinned, tc.installed)
			if got != tc.want || update != tc.wantUpdate {
				t.Errorf("nativeUpdatePlan(%q, %q) = (%q, %v), want (%q, %v)",
					tc.pinned, tc.installed, got, update, tc.want, tc.wantUpdate)
			}
		})
	}
}

// SPX ships its control panel as files beside the binary. They are installed
// once, not per version, since the panel is the same for all of them and the
// ini names a single directory.
func TestUnpackNativePHPInstallsTheSPXWebUI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stage := stageNativeBuild(t, "8.4")
	ui := filepath.Join(stage, "share", "php-spx", "assets", "web-ui")
	if err := os.MkdirAll(ui, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ui, "index.html"), []byte("panel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unpackNativePHP(stage, "8.4", "8.4.25"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.SpxWebUIDir(), "index.html")); err != nil {
		t.Errorf("the SPX control panel was not installed: %v", err)
	}
}

// A build that carries no panel must not wipe the one already installed.
func TestUnpackNativePHPKeepsAnExistingSPXWebUI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.SpxWebUIDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(config.SpxWebUIDir(), "index.html")
	if err := os.WriteFile(kept, []byte("panel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unpackNativePHP(stageNativeBuild(t, "8.4"), "8.4", "8.4.25"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("a build without the panel removed the installed one: %v", err)
	}
}
