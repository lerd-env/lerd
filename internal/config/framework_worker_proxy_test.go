package config

import "testing"

// A site can run an asset server next to a websocket server, so every worker
// that declares a proxy gets one, in a stable order.
func TestDetectProxies_EveryWorkerNotJustTheFirst(t *testing.T) {
	fw := &Framework{Workers: map[string]FrameworkWorker{
		"reverb": {Proxy: &WorkerProxy{Paths: []string{"/app"}, PortEnvKey: "REVERB_SERVER_PORT"}},
		"vite":   {Proxy: &WorkerProxy{Paths: []string{"/build"}, Upstream: "host", Port: "pinned"}},
		"queue":  {Command: "php artisan queue:work"},
	}}

	found := fw.DetectProxies(t.TempDir())
	if len(found) != 2 {
		t.Fatalf("proxies = %d, want 2", len(found))
	}
	if found[0].Worker != "reverb" || found[1].Worker != "vite" {
		t.Errorf("order = %q, %q, want reverb then vite", found[0].Worker, found[1].Worker)
	}
	if !found[1].Proxy.OnHost() || !found[1].Proxy.PinnedPort() {
		t.Errorf("vite proxy = %+v, want a pinned host proxy", found[1].Proxy)
	}
	if found[0].Proxy.OnHost() || found[0].Proxy.PinnedPort() {
		t.Errorf("reverb proxy = %+v, want the container default and an env port", found[0].Proxy)
	}
}
