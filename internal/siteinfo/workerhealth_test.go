package siteinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestHostPortFromURL(t *testing.T) {
	cases := map[string]string{
		"http://[::1]:5173":       "[::1]:5173",
		"http://localhost:5173":   "localhost:5173",
		"https://127.0.0.1:443/x": "127.0.0.1:443",
		"//host:8080":             "host:8080",
		"http://localhost":        "", // no port, nothing to probe
		"not a url":               "",
		"":                        "",
	}
	for in, want := range cases {
		if got := hostPortFromURL(in); got != want {
			t.Errorf("hostPortFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkerServerReachable(t *testing.T) {
	prev := dialProbe
	t.Cleanup(func() { dialProbe = prev })

	dir := t.TempDir()
	hot := filepath.Join(dir, "public", "hot")
	if err := os.MkdirAll(filepath.Dir(hot), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &config.WorkerHealth{URLFile: "public/hot"}

	// No block / empty URLFile: not probeable.
	if _, probed := WorkerServerReachable(dir, nil); probed {
		t.Error("nil health should not be probed")
	}
	if _, probed := WorkerServerReachable(dir, &config.WorkerHealth{}); probed {
		t.Error("empty url_file should not be probed")
	}

	// File missing: not a failure (idle-suspend clears it while briefly up).
	dialProbe = func(string) error { t.Fatal("should not dial when file is absent"); return nil }
	if reachable, probed := WorkerServerReachable(dir, h); probed || reachable {
		t.Errorf("missing hot file: got reachable=%v probed=%v, want false/false", reachable, probed)
	}

	// File present, server accepting: reachable.
	os.WriteFile(hot, []byte("http://[::1]:5173\n"), 0o644)
	var dialed string
	dialProbe = func(addr string) error { dialed = addr; return nil }
	if reachable, probed := WorkerServerReachable(dir, h); !reachable || !probed {
		t.Errorf("reachable server: got reachable=%v probed=%v, want true/true", reachable, probed)
	}
	if dialed != "[::1]:5173" {
		t.Errorf("dialed %q, want [::1]:5173", dialed)
	}

	// File present (stale port), server refusing: unhealthy.
	dialProbe = func(string) error { return os.ErrDeadlineExceeded }
	if reachable, probed := WorkerServerReachable(dir, h); reachable || !probed {
		t.Errorf("dead server: got reachable=%v probed=%v, want false/true", reachable, probed)
	}

	// File present but no port: not probeable.
	os.WriteFile(hot, []byte("http://localhost\n"), 0o644)
	dialProbe = func(string) error { t.Fatal("should not dial an unparseable URL"); return nil }
	if reachable, probed := WorkerServerReachable(dir, h); probed || reachable {
		t.Errorf("portless url: got reachable=%v probed=%v, want false/false", reachable, probed)
	}
}
