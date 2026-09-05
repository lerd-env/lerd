package cli

import (
	"strings"
	"testing"
)

// A native site's CLI runs on the host, so the command is the native binary
// itself with the project as its working directory. No podman, no path
// staging: there is no container boundary to cross.
func TestNativeExecCommand(t *testing.T) {
	cmd := nativeExecCommand("/bin/php-native-8.4", "/srv/shop", []string{"artisan", "migrate"}, "8.4")
	if cmd.Path != "/bin/php-native-8.4" {
		t.Errorf("Path = %q, want the native binary", cmd.Path)
	}
	if cmd.Dir != "/srv/shop" {
		t.Errorf("Dir = %q, want the project dir", cmd.Dir)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "artisan" || cmd.Args[2] != "migrate" {
		t.Errorf("Args = %v, want the binary plus artisan migrate", cmd.Args)
	}
	// php:ini, dumps and xdebug all come from the scan dir, same as in the
	// container. Without it a native CLI would silently ignore them.
	var scan string
	for _, e := range cmd.Env {
		if v, ok := strings.CutPrefix(e, "PHP_INI_SCAN_DIR="); ok {
			scan = v
		}
	}
	if scan == "" {
		t.Error("PHP_INI_SCAN_DIR must be set so php:ini applies to the native CLI")
	}
}

// Extra env from the caller must survive, it carries LERD_SITE and the debug
// bridge wiring.
func TestNativeExecCommandKeepsExtraEnv(t *testing.T) {
	cmd := nativeExecCommand("/bin/php", "/srv/shop", []string{"-v"}, "8.4", "LERD_SITE=shop")
	found := false
	for _, e := range cmd.Env {
		if e == "LERD_SITE=shop" {
			found = true
		}
	}
	if !found {
		t.Error("caller env must be preserved")
	}
}

// The php shim runs anywhere, not only inside a registered site. Under the
// native runtime there is no FPM container to fall back to, so a directory
// that is not a site must still get native PHP rather than starting one.
func TestNativeRuntimeVersionOutsideASite(t *testing.T) {
	nativeMode(t)
	v, ok := nativeRuntimeVersion(t.TempDir())
	if !ok {
		t.Fatal("native mode must claim a directory that is not a site")
	}
	if v == "" {
		t.Error("expected a PHP version to run with")
	}
}

// In container mode the shim keeps its existing behaviour everywhere.
func TestNativeRuntimeVersionInactiveInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, ok := nativeRuntimeVersion(t.TempDir()); ok {
		t.Error("container mode must not divert the shim to native php")
	}
}

// There is no container to open a shell in under the native runtime, and
// ensuring one would start the very thing the mode exists to avoid. Refusing
// with an explanation beats silently resurrecting a container.
func TestShellRefusesUnderNative(t *testing.T) {
	nativeMode(t)
	err := nativeShellRefusal(t.TempDir())
	if err == nil {
		t.Fatal("expected a refusal under the native runtime")
	}
	for _, want := range []string{"native", "container"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Errorf("error should explain the runtime, got: %v", err)
		}
	}
}

func TestShellAllowedInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := nativeShellRefusal(t.TempDir()); err != nil {
		t.Errorf("container mode must allow the shell, got: %v", err)
	}
}

// The native binary's extension set is fixed at build time and there is no
// Alpine image to add packages to, so both commands have to refuse rather than
// appear to work and change nothing.
func TestImageOnlyCommandsRefuseUnderNative(t *testing.T) {
	nativeMode(t)
	for _, cmd := range []string{"php:ext", "php:pkg"} {
		err := nativeImageCommandRefusal(cmd)
		if err == nil {
			t.Errorf("%s must refuse under the native runtime", cmd)
			continue
		}
		if !strings.Contains(err.Error(), cmd) {
			t.Errorf("the refusal should name the command, got: %v", err)
		}
	}
}

func TestImageOnlyCommandsAllowedInContainerMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := nativeImageCommandRefusal("php:ext"); err != nil {
		t.Errorf("container mode must allow php:ext, got: %v", err)
	}
}

// Tinker execs into the FPM container. Under the native runtime that container
// is stopped, and ensuring it would start the one the mode just tore down, so
// tinker has to run the host binary in the project directory instead.
func TestNativeTinkerCommand(t *testing.T) {
	cmd := nativeTinkerCommand("/bin/php-native-8.4", "/srv/shop", "8.4",
		[]string{"-d", "memory_limit=512M", "artisan", "tinker"},
		[]string{"LERD_SITE=shop"})
	if cmd.Dir != "/srv/shop" {
		t.Errorf("Dir = %q, want the project dir", cmd.Dir)
	}
	if cmd.Path != "/bin/php-native-8.4" {
		t.Errorf("Path = %q, want the native binary", cmd.Path)
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != "-d" {
		t.Errorf("Args = %v, want the php arguments preserved", cmd.Args)
	}
	var sawSite, sawScan bool
	for _, e := range cmd.Env {
		if e == "LERD_SITE=shop" {
			sawSite = true
		}
		if strings.HasPrefix(e, "PHP_INI_SCAN_DIR=") {
			sawScan = true
		}
	}
	if !sawSite {
		t.Error("caller env must survive; it carries the site for the debug bridge")
	}
	if !sawScan {
		t.Error("PHP_INI_SCAN_DIR must be set so php:ini applies to tinker too")
	}
}
