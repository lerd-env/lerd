package cli

import (
	"strconv"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// stageSites writes a registry holding the given sites and points lerd at it.
func stageSites(t *testing.T, sites ...config.Site) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	for _, s := range sites {
		if err := config.AddSite(s); err != nil {
			t.Fatal(err)
		}
	}
}

// A pinned port is allocated once and then reused, so the vhost and the worker
// keep agreeing across restarts.
func TestPinnedWorkerPort_StableAcrossCalls(t *testing.T) {
	stageSites(t, config.Site{Name: "app", Path: t.TempDir()})

	first := pinnedWorkerPort("app", "vite", 5173)
	if first < 5173 {
		t.Fatalf("port = %d, want the default or above", first)
	}
	if second := pinnedWorkerPort("app", "vite", 5173); second != first {
		t.Errorf("second call = %d, want the recorded %d", second, first)
	}
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	if stored.WorkerPorts["vite"] != first {
		t.Errorf("registry = %d, want %d", stored.WorkerPorts["vite"], first)
	}
}

// Another site's pinned port and another site's dev server are both out of
// bounds: two servers on one port is the failure this allocation exists to
// prevent.
func TestPinnedWorkerPort_AvoidsPortsOtherSitesHold(t *testing.T) {
	stageSites(t,
		config.Site{Name: "other", Path: t.TempDir(), WorkerPorts: map[string]int{"vite": 5173}},
		config.Site{Name: "third", Path: t.TempDir(), DevServerPort: 5174},
		config.Site{Name: "app", Path: t.TempDir()},
	)

	got := pinnedWorkerPort("app", "vite", 5173)
	if got == 5173 || got == 5174 {
		t.Errorf("port = %d, want one no other site holds", got)
	}
}

// The port reaches the process through the key the definition names, in front
// of the command, since the command may take no port flag at all.
func TestWithPinnedWorkerPort_PrefixesTheCommand(t *testing.T) {
	stageSites(t, config.Site{Name: "app", Path: t.TempDir()})

	w := config.FrameworkWorker{
		Command: "php artisan vite:watch theme",
		Host:    true,
		Proxy:   &config.WorkerProxy{Paths: []string{"/build"}, Upstream: "host", Port: "pinned", PortEnvKey: "VITE_PORT", DefaultPort: 5173},
	}
	got := withPinnedWorkerPort("app", "vite", w, w.Command)
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	// env(1), not a bare assignment: a host worker's command is spliced after
	// the version manager's exec, where a prefix would be read as a program name.
	want := "env VITE_PORT=" + strconv.Itoa(stored.WorkerPorts["vite"]) + " php artisan vite:watch theme"
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

// A proxy that reads its port from .env keeps the old behaviour and is left
// alone here.
func TestWithPinnedWorkerPort_LeavesEnvPortWorkersAlone(t *testing.T) {
	stageSites(t, config.Site{Name: "app", Path: t.TempDir()})

	w := config.FrameworkWorker{
		Command: "php artisan reverb:start",
		Proxy:   &config.WorkerProxy{Paths: []string{"/app"}, PortEnvKey: "REVERB_SERVER_PORT", DefaultPort: 8080},
	}
	if got := withPinnedWorkerPort("app", "reverb", w, w.Command); got != w.Command {
		t.Errorf("command = %q, want it unchanged", got)
	}
}

// A command that wants the port as an argument writes it as $KEY. The shell
// would expand that before env(1) sets anything, so lerd fills it in itself.
func TestWithPinnedWorkerPort_FillsThePortIntoTheCommand(t *testing.T) {
	stageSites(t, config.Site{Name: "app", Path: t.TempDir()})

	w := config.FrameworkWorker{
		Command: "python3 -m http.server ${PROBE_PORT} --bind $PROBE_PORT",
		Host:    true,
		Proxy:   &config.WorkerProxy{Paths: []string{"/probe"}, Upstream: "host", Port: "pinned", PortEnvKey: "PROBE_PORT", DefaultPort: 4321},
	}
	got := withPinnedWorkerPort("app", "probe", w, w.Command)
	stored, err := config.FindSite("app")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(stored.WorkerPorts["probe"])
	want := "env PROBE_PORT=" + port + " python3 -m http.server " + port + " --bind " + port
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}
