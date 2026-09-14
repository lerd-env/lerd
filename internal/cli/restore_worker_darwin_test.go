//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The unit is rewritten on every `lerd start`, so the port a proxied worker
// answers on has to be rebuilt with it. Without this the worker comes back on
// its own default while the vhost still proxies to the port lerd picked, and
// the site's page is refused by a server that is not there.
func TestRestoreWorker_carriesTheProxyPort(t *testing.T) {
	registerSite(t, "ws", "/p/ws")
	swapMgr(t, &trackingHostMgr{})
	swapDaemonReload(t)

	w := config.FrameworkWorker{
		Command: "php artisan reverb:start",
		Proxy:   &config.WorkerProxy{PortEnvKey: "REVERB_SERVER_PORT", DefaultPort: 8080},
	}
	restoreWorker("ws", "/p/ws", "8.4", "reverb", w)

	body, err := os.ReadFile(filepath.Join(config.RunDir(), "workers", "lerd-reverb-ws.sh"))
	if err != nil {
		t.Fatalf("reading guard script: %v", err)
	}
	if !strings.Contains(string(body), "--port=") {
		t.Errorf("restored worker command carries no proxy port:\n%s", body)
	}
}
