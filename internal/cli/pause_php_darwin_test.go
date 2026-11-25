package cli

import (
	"errors"
	"io"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Under the native runtime there is no container to ensure: resuming fetches and
// starts the host build, and a failed fetch is reported.
func TestEnsureSitePHP_native(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	containers := stubResumePHP(t, nil, true)
	prevInstall, prevStart := nativeInstallFn, nativeStartFn
	t.Cleanup(func() { nativeInstallFn, nativeStartFn = prevInstall, prevStart })
	var installErr error
	var started []string
	nativeInstallFn = func(string, io.Writer) error { return installErr }
	nativeStartFn = func(v string) error { started = append(started, v); return nil }

	if err := ensureSitePHP("8.3"); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0] != "8.3" || len(*containers) != 0 {
		t.Errorf("started %v, containers %v, want only native 8.3", started, *containers)
	}
	installErr = errors.New("offline")
	if err := ensureSitePHP("8.3"); err == nil {
		t.Error("a failed native install must not be swallowed")
	}
}
