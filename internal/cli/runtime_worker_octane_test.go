package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FrankenPHP worker mode runs the site through artisan octane, so a project
// without laravel/octane has nothing to run. Before this the switch reported
// success and the container crash-looped on "no commands defined in the octane
// namespace" with the site stuck at 502.
func TestWorkerModeRefusedWithoutOctane(t *testing.T) {
	dir := t.TempDir()
	writeComposer(t, dir, `{"require":{"laravel/framework":"^13.0"}}`)

	err := requireOctaneForWorkerMode(dir)
	if err == nil {
		t.Fatal("expected worker mode to be refused without laravel/octane")
	}
	if !strings.Contains(err.Error(), "laravel/octane") {
		t.Errorf("error should name the missing package, got: %v", err)
	}
	if !strings.Contains(err.Error(), "composer require") {
		t.Errorf("error should say how to install it, got: %v", err)
	}
}

func TestWorkerModeAllowedWithOctane(t *testing.T) {
	dir := t.TempDir()
	writeComposer(t, dir, `{"require":{"laravel/framework":"^13.0","laravel/octane":"^2.0"}}`)

	if err := requireOctaneForWorkerMode(dir); err != nil {
		t.Fatalf("worker mode should be allowed with octane installed: %v", err)
	}
}

// The check reads what composer actually installed, so a package pulled in
// underneath another one still counts.
func TestWorkerModeAllowedWhenOctaneOnlyInLock(t *testing.T) {
	dir := t.TempDir()
	writeComposer(t, dir, `{"require":{"laravel/framework":"^13.0"}}`)
	if err := os.WriteFile(filepath.Join(dir, "composer.lock"),
		[]byte(`{"packages":[{"name":"laravel/octane","version":"2.0.0"}],"packages-dev":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := requireOctaneForWorkerMode(dir); err != nil {
		t.Fatalf("worker mode should be allowed when the lock carries octane: %v", err)
	}
}

func writeComposer(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
