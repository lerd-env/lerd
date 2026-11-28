package tools

import (
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stubBrewNginx answers nginx -v and brew info the way the real tools do.
func stubBrewNginx(t *testing.T, installed, stable string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &config.GlobalConfig{}
	cfg.PHP.Runtime = config.PHPRuntimeNative
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.SetNginxOnHost(true); err != nil {
		t.Fatal(err)
	}
	prevLook, prevRun := brewLook, brewRun
	t.Cleanup(func() { brewLook, brewRun = prevLook, prevRun })
	brewLook = func(name string) (string, bool) { return "/opt/homebrew/bin/" + name, true }
	brewRun = func(bin string, args ...string) ([]byte, error) {
		if args[0] == "-v" {
			return []byte("nginx version: nginx/" + installed + "\n"), nil
		}
		if stable == "" {
			return nil, errors.New("offline")
		}
		return []byte(`{"formulae":[{"versions":{"stable":"` + stable + `"}}]}`), nil
	}
}

// lerd installs nginx through Homebrew when PHP moves to the host, so it is
// listed with lerd's tools, at the version Homebrew would upgrade it to.
func TestHostNginxStatus_listsHomebrewsNginx(t *testing.T) {
	stubBrewNginx(t, "1.31.10", "1.31.11")
	s, ok := hostNginxStatus()
	if !ok {
		t.Fatal("host nginx not listed beside native PHP")
	}
	if s.Name != "nginx" || s.Installed != "1.31.10" || s.Pinned != "1.31.11" || !s.UpdateAvailable {
		t.Errorf("status = %+v, want 1.31.10 with 1.31.11 available", s)
	}
}

func TestHostNginxStatus_currentWhenHomebrewAgrees(t *testing.T) {
	stubBrewNginx(t, "1.31.10", "1.31.10")
	if s, _ := hostNginxStatus(); s.UpdateAvailable {
		t.Errorf("status = %+v, want no update", s)
	}
}

// Until nginx runs on the host a Homebrew nginx is the user's, which lerd
// does not upgrade, even beside native PHP.
func TestHostNginxStatus_hiddenWithContainerNginx(t *testing.T) {
	stubBrewNginx(t, "1.31.10", "1.31.10")
	_ = config.SetNginxOnHost(false)
	if _, ok := hostNginxStatus(); ok {
		t.Error("host nginx listed while nginx runs in its container")
	}
}

// Podman is listed for its version, the first thing to read when a machine
// misbehaves; lerd never upgrades it, since a new major can break the VM.
func TestPodmanStatus_listsTheVersionWithoutAnUpdate(t *testing.T) {
	prevLook, prevRun := brewLook, brewRun
	t.Cleanup(func() { brewLook, brewRun = prevLook, prevRun })
	brewLook = func(name string) (string, bool) { return "/opt/homebrew/bin/" + name, true }
	brewRun = func(string, ...string) ([]byte, error) { return []byte("podman version 6.1.4\n"), nil }
	s, ok := podmanStatus()
	if !ok || s.Name != "podman" || s.Installed != "6.1.4" || !s.External || s.UpdateAvailable {
		t.Errorf("podmanStatus = %+v, %v; want 6.1.4, external, no update", s, ok)
	}
}
