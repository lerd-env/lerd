package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// Jump needs a host PHP because NativePHP's bundled binary lacks posix and
// pcntl. The native runtime already installs a full static PHP for the same
// versions, so downloading a second, slimmer one is pure duplication.
func TestHostPHPPrefersAnInstalledNativeBinary(t *testing.T) {
	tmp := t.TempDir()
	native := filepath.Join(tmp, "php-native-8.4")
	if err := os.WriteFile(native, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	got, ok := installedNativeHostPHP("8.4", func(string) string { return native })
	if !ok {
		t.Fatal("an installed native binary should be used")
	}
	if got != native {
		t.Errorf("got %q, want the native binary", got)
	}
}

// With no native binary the caller falls back to the pinned download, so
// container-mode installs keep working exactly as before.
func TestHostPHPFallsBackWhenNoNativeBinary(t *testing.T) {
	if _, ok := installedNativeHostPHP("8.4", func(string) string {
		return filepath.Join(t.TempDir(), "absent")
	}); ok {
		t.Error("a missing native binary must fall back to the pinned download")
	}
}

// A path that exists but cannot be executed is not usable either.
func TestHostPHPRejectsANonExecutable(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "php-native-8.4")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := installedNativeHostPHP("8.4", func(string) string { return p }); ok {
		t.Error("a non-executable file must not be treated as installed")
	}
}
