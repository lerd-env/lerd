package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// stubWebServerSwitch replaces the steps that touch podman and the network,
// recording restarts and answering health with the given errors in turn.
func stubWebServerSwitch(t *testing.T, vm bool, health ...error) *int {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	restarts := new(int)
	prevRestart, prevHealth, prevPull, prevVM := webServerRestart, webServerHealthy, webServerPull, webServerOnVM
	t.Cleanup(func() {
		webServerRestart, webServerHealthy, webServerPull, webServerOnVM = prevRestart, prevHealth, prevPull, prevVM
	})
	webServerRestart = func() error { *restarts++; return nil }
	webServerHealthy = func() error {
		if len(health) == 0 {
			return nil
		}
		err := health[0]
		health = health[1:]
		return err
	}
	webServerPull = func(string) error { return nil }
	webServerOnVM = func() bool { return vm }
	prevStop := webServerStop
	t.Cleanup(func() { webServerStop = prevStop })
	webServerStop = func(string) {}
	return restarts
}

func currentWebServer(t *testing.T) string {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	kind, _ := cfg.WebServerKind()
	return kind
}

func readUnit(t *testing.T) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(config.QuadletDir(), podman.WebServerUnit()+".container"))
	return string(data)
}

func unitFileExists(name string) bool {
	_, err := os.Stat(filepath.Join(config.QuadletDir(), name+".container"))
	return err == nil
}

func TestSwitchWebServer_toGoRewritesTheUnitAndRestarts(t *testing.T) {
	restarts := stubWebServerSwitch(t, false)
	if err := SwitchWebServer("go", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if currentWebServer(t) != config.WebServerGo || *restarts != 1 || !strings.Contains(readUnit(t), "lerd web-serve") {
		t.Fatalf("kind %s restarts %d unit:\n%s", currentWebServer(t), *restarts, readUnit(t))
	}
	// nginx is gone, not merely idle: its unit no longer exists to start.
	if unitFileExists("lerd-nginx") || !unitFileExists("lerd-web") {
		t.Errorf("units: nginx %v, web %v", unitFileExists("lerd-nginx"), unitFileExists("lerd-web"))
	}
}

// A switch whose server never answers is rolled back, so the sites keep the
// server they had rather than being left on one that does not start.
func TestSwitchWebServer_unhealthyRollsBack(t *testing.T) {
	restarts := stubWebServerSwitch(t, false, errors.New("nothing on :80"))
	err := SwitchWebServer("go", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "nothing on :80") {
		t.Fatalf("err %v", err)
	}
	if currentWebServer(t) != config.WebServerNginx || *restarts != 2 || strings.Contains(readUnit(t), "lerd web-serve") {
		t.Fatalf("not rolled back: kind %s restarts %d unit:\n%s", currentWebServer(t), *restarts, readUnit(t))
	}
	if unitFileExists("lerd-web") {
		t.Error("the failed server's unit survived the rollback")
	}
}

func TestSwitchWebServer_refusals(t *testing.T) {
	t.Run("unknown server", func(t *testing.T) {
		stubWebServerSwitch(t, false)
		if err := SwitchWebServer("apache", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "apache") {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("macOS", func(t *testing.T) {
		stubWebServerSwitch(t, true)
		if err := SwitchWebServer("go", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "macOS") {
			t.Fatalf("err %v", err)
		}
	})
	// A raw nginx override cannot follow the site, so the switch is refused
	// and names the file rather than dropping what the user wrote.
	t.Run("nginx override", func(t *testing.T) {
		stubWebServerSwitch(t, false)
		override := filepath.Join(config.NginxCustomD(), "shop.test.conf")
		os.MkdirAll(filepath.Dir(override), 0o755)
		os.WriteFile(override, []byte("client_max_body_size 100m;\n"), 0o644)
		err := SwitchWebServer("go", &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "shop.test.conf") {
			t.Fatalf("err %v", err)
		}
		if currentWebServer(t) != config.WebServerNginx {
			t.Fatal("switched despite the override")
		}
	})
}

func TestSwitchWebServer_backToNginxRewritesItsUnit(t *testing.T) {
	stubWebServerSwitch(t, false)
	if err := SwitchWebServer("go", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := SwitchWebServer("nginx", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if currentWebServer(t) != config.WebServerNginx || !strings.Contains(readUnit(t), "nginx:alpine") {
		t.Fatalf("kind %s unit:\n%s", currentWebServer(t), readUnit(t))
	}
	if unitFileExists("lerd-web") {
		t.Error("lerd's own server's unit survived the switch back")
	}
	if _, err := os.Stat(filepath.Join(config.NginxConfD(), "lerd.localhost.conf")); err != nil {
		t.Errorf("nginx config not rebuilt: %v", err)
	}
}
