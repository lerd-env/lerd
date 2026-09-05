package nativephp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Each PHP version gets its own FPM listener, derived from the version so the
// port is stable across restarts without any state to persist.
func TestPortFor(t *testing.T) {
	cases := []struct {
		version string
		want    int
	}{
		{"8.1", 9481},
		{"8.4", 9484},
		{"8.5", 9485},
		{"7.4", 9474},
	}
	for _, c := range cases {
		got, err := PortFor(c.version)
		if err != nil {
			t.Fatalf("PortFor(%q): %v", c.version, err)
		}
		if got != c.want {
			t.Errorf("PortFor(%q) = %d, want %d", c.version, got, c.want)
		}
	}
}

func TestPortForRejectsGarbage(t *testing.T) {
	for _, v := range []string{"", "8", "abc", "8.x", "8.1.2"} {
		if _, err := PortFor(v); err == nil {
			t.Errorf("PortFor(%q) should have failed", v)
		}
	}
}

// Two versions must never collide, or one FPM silently shadows the other.
func TestPortForIsUnique(t *testing.T) {
	seen := map[int]string{}
	for _, v := range []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6"} {
		p, err := PortFor(v)
		if err != nil {
			t.Fatalf("PortFor(%q): %v", v, err)
		}
		if prev, dup := seen[p]; dup {
			t.Fatalf("port %d shared by %s and %s", p, prev, v)
		}
		seen[p] = v
	}
}

// A native FPM must load the same conf.d files the containers bind-mount, in the
// same order, or php:ini, dumps, the debug window and xdebug all stop working
// the moment a site switches runtime.
func TestIniScanDirsOrdering(t *testing.T) {
	dirs := IniScanDirs("8.4")
	if len(dirs) < 2 {
		t.Fatalf("expected several scan dirs, got %v", dirs)
	}
	wantOrder := []string{"mail", "shared", "devtools", "dumps", "8.4"}
	var idx []int
	for _, want := range wantOrder {
		found := -1
		for i, d := range dirs {
			if filepathBase(d) == want {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("scan dirs %v missing %q", dirs, want)
		}
		idx = append(idx, found)
	}
	for i := 1; i < len(idx); i++ {
		if idx[i] <= idx[i-1] {
			t.Errorf("scan dir %q must come after %q; got %v", wantOrder[i], wantOrder[i-1], dirs)
		}
	}
}

func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func TestFPMConfig(t *testing.T) {
	got, err := FPMConfig("8.4", "/tmp/lerd-native-84.log")
	if err != nil {
		t.Fatalf("FPMConfig: %v", err)
	}
	// nginx dials in from the podman VM, so a loopback-only bind would refuse
	// every request.
	if !strings.Contains(got, "listen = 9484") {
		t.Errorf("want a listener on the version's port, got:\n%s", got)
	}
	if strings.Contains(got, "listen = 127.0.0.1") {
		t.Error("a loopback-only bind is unreachable from the nginx container")
	}
	// launchd supervises the process, so FPM must stay in the foreground.
	if !strings.Contains(got, "daemonize = no") {
		t.Error("FPM must not daemonize under launchd")
	}
	// The site's own env reaches the app through the FPM process environment.
	if !strings.Contains(got, "clear_env = no") {
		t.Error("clear_env must stay off")
	}
	if !strings.Contains(got, "/tmp/lerd-native-84.log") {
		t.Error("error_log should point at the given path")
	}
}

func TestFPMConfigRejectsBadVersion(t *testing.T) {
	if _, err := FPMConfig("nope", "/tmp/x.log"); err == nil {
		t.Error("FPMConfig should reject an unparseable version")
	}
}

// Regenerating must not churn the file, or every call restarts FPM for nothing.
func TestFPMConfigIsDeterministic(t *testing.T) {
	a, _ := FPMConfig("8.4", "/tmp/x.log")
	b, _ := FPMConfig("8.4", "/tmp/x.log")
	if a != b {
		t.Error("FPMConfig must be deterministic")
	}
}

func TestBinaryPathIsVersioned(t *testing.T) {
	a := BinaryPath("8.4")
	b := BinaryPath("8.5")
	if a == b {
		t.Fatal("each PHP version needs its own binary path")
	}
	if !strings.HasSuffix(a, "php-native-8.4") {
		t.Errorf("BinaryPath(8.4) = %q, want a php-native-8.4 suffix", a)
	}
	// Must never collide with the container shim named "php" in the same dir.
	if filepathBase(a) == "php" {
		t.Error("native binary must not be named php; that is the container shim")
	}
}

// Switching a site to native with no binary installed must say so, not start a
// listener that nothing answers on.
func TestEnsureInstalledReportsMissingBinary(t *testing.T) {
	err := EnsureInstalled("8.4", "/nonexistent/php-native-8.4")
	if err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if !strings.Contains(err.Error(), "8.4") {
		t.Errorf("error should name the version, got: %v", err)
	}
}

// FPM serves requests, the CLI runs artisan and composer. They are separate
// binaries and must not share a path.
func TestFPMBinaryPathDiffersFromCLI(t *testing.T) {
	if BinaryPath("8.4") == FPMBinaryPath("8.4") {
		t.Fatal("cli and fpm binaries must not share a path")
	}
	if !strings.Contains(FPMBinaryPath("8.4"), "fpm") {
		t.Errorf("FPMBinaryPath should be identifiable, got %q", FPMBinaryPath("8.4"))
	}
}

// Workers run "php artisan ..." through a shell, so a dir holding a php ->
// native-CLI symlink goes on PATH ahead of BinDir, whose php is the container
// shim. Without this a native site's workers would silently run in a container.
func TestShimDirIsPerVersionAndSeparateFromBinDir(t *testing.T) {
	d84 := ShimDir("8.4")
	if d84 == ShimDir("8.5") {
		t.Error("shim dir must be per version")
	}
	if d84 == config.BinDir() {
		t.Error("shim dir must not be BinDir, whose php is the container shim")
	}
}

func TestEnsureShimCreatesAndRepoints(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	first := filepath.Join(tmp, "php-native-8.4")
	if err := os.WriteFile(first, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	dir, err := EnsureShim("8.4", first)
	if err != nil {
		t.Fatalf("EnsureShim: %v", err)
	}
	got, err := os.Readlink(filepath.Join(dir, "php"))
	if err != nil || got != first {
		t.Fatalf("shim link = %q (%v), want %q", got, err, first)
	}

	// A reinstall points the same link somewhere new rather than failing.
	second := filepath.Join(tmp, "php-native-8.4.new")
	if err := os.WriteFile(second, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureShim("8.4", second); err != nil {
		t.Fatalf("EnsureShim repoint: %v", err)
	}
	if got, _ := os.Readlink(filepath.Join(dir, "php")); got != second {
		t.Errorf("shim link = %q, want %q after repoint", got, second)
	}
}

// The conf.d files the containers mount name container paths (the dump bridge
// is auto-prepended from /usr/local/etc/lerd). PHP fatals on an
// auto_prepend_file it cannot open, so the override re-points the bridge and
// its assets at the host copies rather than turning capture off.
func TestOverrideIniPointsTheBridgeAtHostPaths(t *testing.T) {
	got := overrideIni("8.4")
	if strings.Contains(got, "/usr/local/etc/lerd") {
		t.Errorf("override must not carry container paths:\n%s", got)
	}
	for _, want := range []string{"auto_prepend_file=", "dump-bridge.php", "lerd.assets_dir=", "lerd.dump_host=tcp://127.0.0.1:9913"} {
		if !strings.Contains(got, want) {
			t.Errorf("override missing %q:\n%s", want, got)
		}
	}
	// An empty prepend would disable dump()/dd() entirely.
	if strings.Contains(got, "auto_prepend_file=\n") {
		t.Errorf("the prepend must point somewhere, not be cleared:\n%s", got)
	}
}

// It must be scanned after the copied container inis or it cannot win.
func TestOverrideDirIsScannedLast(t *testing.T) {
	dirs := IniScanDirs("8.4")
	last := dirs[len(dirs)-1]
	if last != OverrideDir("8.4") {
		t.Errorf("last scan dir = %q, want the override dir %q", last, OverrideDir("8.4"))
	}
}

// The settings page lists what is installed. Under the native runtime that is
// the binaries on disk, not the container images, or it offers versions that
// cannot serve anything.
func TestListInstalledFindsNativeBinaries(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	binDir := config.BinDir()
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Both SAPIs are needed for a version to be usable.
	for _, name := range []string{"php-native-8.3", "php-native-fpm-8.3", "php-native-8.4", "php-native-fpm-8.4"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// A CLI with no FPM cannot serve, so it must not be listed.
	if err := os.WriteFile(filepath.Join(binDir, "php-native-8.5"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	got := ListInstalled()
	want := []string{"8.3", "8.4"}
	if len(got) != len(want) {
		t.Fatalf("ListInstalled = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListInstalled = %v, want %v", got, want)
		}
	}
}

// Query capture is an engine-level extension. When the build ships one next to
// the binary the runtime has to load it, and when it does not the ini must stay
// silent rather than naming a file PHP would warn about on every request.
func TestOverrideLoadsDevtoolsOnlyWhenPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)

	if got := overrideIni("8.4"); strings.Contains(got, "lerd_devtools") {
		t.Errorf("no extension present, so nothing should be loaded:\n%s", got)
	}

	so := filepath.Join(ModulesDir("8.4"), "lerd_devtools-8.4.so")
	if err := os.MkdirAll(filepath.Dir(so), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(so, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got := overrideIni("8.4")
	// lerd_devtools registers a zend_module_entry, so it loads with extension=.
	// zend_extension= is for the other kind and PHP refuses the module outright.
	if !strings.Contains(got, "extension="+so) {
		t.Errorf("a present extension must be loaded:\n%s", got)
	}
	if strings.Contains(got, "zend_extension="+so) {
		t.Errorf("lerd_devtools is a module, not a zend extension:\n%s", got)
	}
}
