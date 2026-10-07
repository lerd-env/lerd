package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
)

func lerdVhostPath() string {
	return filepath.Join(config.NginxConfD(), "lerd.localhost.conf")
}

// A vhost that is not on disk yet has to be written, and the caller told so, or
// a service installed after the last `lerd start` keeps serving nothing.
func TestSyncLerdVhost_WritesWhenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.NginxConfD(), 0o755); err != nil {
		t.Fatal(err)
	}

	changed, err := SyncLerdVhost()
	if err != nil {
		t.Fatalf("SyncLerdVhost: %v", err)
	}
	if !changed {
		t.Error("a vhost written for the first time is a change")
	}
	if _, err := os.Stat(lerdVhostPath()); err != nil {
		t.Errorf("vhost not on disk: %v", err)
	}
}

// Nothing changed means nothing written, so a reload is not provoked on every
// service operation that leaves the vhost saying exactly what it already said.
func TestSyncLerdVhost_NoWriteWhenIdentical(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.NginxConfD(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncLerdVhost(); err != nil {
		t.Fatalf("first SyncLerdVhost: %v", err)
	}
	before, err := os.Stat(lerdVhostPath())
	if err != nil {
		t.Fatal(err)
	}

	changed, err := SyncLerdVhost()
	if err != nil {
		t.Fatalf("second SyncLerdVhost: %v", err)
	}
	if changed {
		t.Error("an unchanged vhost must not report a change")
	}
	after, err := os.Stat(lerdVhostPath())
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged vhost must not be rewritten")
	}
}

// The case that started this: a vhost from before the service existed serves
// none of its dashboard's paths, and every request to the console is closed by
// the catch-all until something rewrites the file.
func TestSyncLerdVhost_RestoresAStaleVhost(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(config.NginxConfD(), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := "server {\n    listen 80;\n    server_name lerd.localhost;\n    location / {\n        return 444;\n    }\n}\n"
	if err := os.WriteFile(lerdVhostPath(), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := SyncLerdVhost()
	if err != nil {
		t.Fatalf("SyncLerdVhost: %v", err)
	}
	if !changed {
		t.Fatal("a stale vhost must be reported as changed")
	}
	got, err := os.ReadFile(lerdVhostPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == stale {
		t.Error("the stale vhost is still on disk")
	}
	// The mounts themselves come from the installed presets, which a temp home
	// has none of; what has to hold here is that the file is the one lerd would
	// write now, with its own allowlist back in place.
	if !strings.Contains(string(got), "/_svc/") {
		t.Error("the regenerated vhost does not serve the dashboard prefix")
	}
}

// A dashboard embedded under /_svc/ pushes live updates over a websocket, as
// Mailpit does, so the handshake headers have to reach lerd-ui on both shapes.
func TestLerdVhost_SvcForwardsWebsocketUpgrade(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	saved := platform.Current
	t.Cleanup(func() { platform.Current = saved })

	for _, vm := range []bool{false, true} {
		platform.Current.UsesMachineVM = vm
		content, err := renderLerdVhost()
		if err != nil {
			t.Fatalf("vm=%v: %v", vm, err)
		}
		// Server level, since a proxy_set_header inside a location would drop
		// the inherited Host and forwarding headers.
		server := content[:strings.Index(content, "location")]
		for _, want := range []string{"proxy_set_header Upgrade $http_upgrade;", `proxy_set_header Connection "upgrade";`} {
			if !strings.Contains(server, want) {
				t.Errorf("vm=%v: vhost is missing %q:\n%s", vm, want, server)
			}
		}
	}
}
