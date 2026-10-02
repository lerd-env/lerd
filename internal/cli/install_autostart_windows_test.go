//go:build windows

package cli

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/geodro/lerd/internal/config"
)

func TestAutostartCommandQuotesThePath(t *testing.T) {
	got := autostartCommand(`C:\Users\me\My Tools\lerd.exe`)
	want := `"C:\Users\me\My Tools\lerd.exe" start`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// isolateRunKey points the autostart code at a throwaway key, so a test never
// touches the user's real login entries.
func isolateRunKey(t *testing.T) {
	t.Helper()
	prev := runKeyPath
	runKeyPath = `Software\lerd-test-` + filepath.Base(t.TempDir()) + `\Run`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, runKeyPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, filepath.Dir(runKeyPath))
		runKeyPath = prev
	})
}

// autostartOff isolates the config and records the autostart preference in it.
func autostartOff(t *testing.T, disabled bool) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Autostart.Disabled = disabled
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestAutostartRoundTrip(t *testing.T) {
	isolateRunKey(t)
	autostartOff(t, false)
	installAutostart()
	t.Cleanup(func() { _ = removeAutostart() })
	v, err := readRunValue()
	if err != nil || v == "" {
		t.Fatalf("value not written: %q %v", v, err)
	}
	if err := removeAutostart(); err != nil {
		t.Fatal(err)
	}
	if _, err := readRunValue(); err == nil {
		t.Error("value still present after removeAutostart")
	}
}

func readRunValue() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValueName)
	return v, err
}

// `lerd autostart disable` must hold across a reinstall: install used to put the
// Run entry back regardless.
func TestInstallAutostartRespectsDisable(t *testing.T) {
	isolateRunKey(t)
	autostartOff(t, true)
	installAutostart()
	if v, err := readRunValue(); err == nil {
		t.Errorf("Run entry written while autostart is disabled: %q", v)
	}
}

func TestSyncLoginAutostartAddsAndRemoves(t *testing.T) {
	isolateRunKey(t)
	autostartOff(t, false)
	syncLoginAutostart(false)
	if _, err := readRunValue(); err != nil {
		t.Fatalf("enable left no Run entry: %v", err)
	}
	syncLoginAutostart(true)
	if _, err := readRunValue(); err == nil {
		t.Error("disable left the Run entry behind")
	}
}
