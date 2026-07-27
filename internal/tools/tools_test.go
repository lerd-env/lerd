package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// offline points the manifest fetch at a dead endpoint so Load exercises the
// embedded fallback without touching the network, and isolates the disk cache
// and bin dir in a temp XDG data home.
func offline(t *testing.T) {
	t.Helper()
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

var platforms = []struct{ goos, goarch string }{
	{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"},
}

func TestEmbedded_ResolvesEveryToolOnEveryPlatform(t *testing.T) {
	offline(t)
	m := Load(context.Background())
	for _, name := range []string{"composer", "fnm", "mkcert"} {
		tool, ok := m.Tools[name]
		if !ok {
			t.Fatalf("embedded manifest is missing %s", name)
		}
		for _, p := range platforms {
			url, err := m.URL(name, p.goos, p.goarch)
			if err != nil {
				t.Errorf("URL(%s, %s/%s): %v", name, p.goos, p.goarch, err)
				continue
			}
			if strings.Contains(url, "latest") || strings.Contains(url, "stable") {
				t.Errorf("%s URL %q floats on a channel instead of pinning a version", name, url)
			}
			if !strings.Contains(url, tool.Version) {
				t.Errorf("%s URL %q does not contain pinned version %q", name, url, tool.Version)
			}
		}
	}
}

func TestURL_ExpandsAssetTemplate(t *testing.T) {
	offline(t)
	m := Load(context.Background())
	got, err := m.URL("mkcert", "linux", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	v := m.Tools["mkcert"].Version
	want := "https://github.com/FiloSottile/mkcert/releases/download/" + v + "/mkcert-" + v + "-linux-arm64"
	if got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}

func TestURL_UnknownToolAndPlatform(t *testing.T) {
	offline(t)
	m := Load(context.Background())
	if _, err := m.URL("nope", "linux", "amd64"); err == nil {
		t.Error("expected error for unknown tool")
	}
	if _, err := m.URL("fnm", "plan9", "386"); err == nil {
		t.Error("expected error for unsupported platform")
	}
}

func TestLoad_OverlaysPublishedPins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "tools:\n  composer:\n    version: \"9.9.9\"\n    url: https://getcomposer.org/download/{version}/composer.phar\n")
	}))
	defer srv.Close()
	t.Setenv("LERD_TOOLS_URL", srv.URL)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	m := Load(context.Background())
	url, err := m.URL("composer", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "9.9.9") {
		t.Errorf("published composer pin not applied, URL = %q", url)
	}
	// Tools absent from the published manifest keep their embedded pins.
	if _, err := m.URL("mkcert", "linux", "amd64"); err != nil {
		t.Errorf("embedded mkcert pin lost after overlay: %v", err)
	}
}

func TestLoad_RejectsInvalidPublishedValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "tools:\n  composer:\n    version: \"../../evil\"\n    url: https://getcomposer.org/download/{version}/composer.phar\n  fnm:\n    version: v1.0.0\n    url: http://insecure.example/{version}\n")
	}))
	defer srv.Close()
	t.Setenv("LERD_TOOLS_URL", srv.URL)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	m := Load(context.Background())
	if v := m.Tools["composer"].Version; v != embeddedManifest().Tools["composer"].Version {
		t.Errorf("path-traversal version overrode the embedded pin: %q", v)
	}
	if u := m.Tools["fnm"].URL; !strings.HasPrefix(u, "https://") {
		t.Errorf("non-HTTPS URL overrode the embedded pin: %q", u)
	}
}

func TestLoad_CachesPublishedManifestOnDisk(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, "tools:\n  composer:\n    version: \"9.9.9\"\n    url: https://getcomposer.org/download/{version}/composer.phar\n")
	}))
	t.Setenv("LERD_TOOLS_URL", srv.URL)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	Load(context.Background())
	srv.Close() // second Load must not need the network
	m := Load(context.Background())
	if calls.Load() != 1 {
		t.Errorf("fetches = %d, want 1 (second load served from cache)", calls.Load())
	}
	if v := m.Tools["composer"].Version; v != "9.9.9" {
		t.Errorf("cached composer pin = %q, want 9.9.9", v)
	}
}

func TestLoad_FailedFetchBacksOff(t *testing.T) {
	offline(t)
	Load(context.Background())
	// The failure marker must make the next Load skip the (dead) endpoint
	// instead of stalling on it again within the TTL.
	if _, err := os.Stat(manifestCachePath()); err != nil {
		t.Fatalf("no backoff marker written after failed fetch: %v", err)
	}
	start := time.Now()
	Load(context.Background())
	if time.Since(start) > 2*time.Second {
		t.Error("second Load after a failed fetch still waited on the network")
	}
}

func TestInstalledVersion_StampAndProbe(t *testing.T) {
	offline(t)
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	// Missing binary reports empty.
	if v := InstalledVersion("mkcert"); v != "" {
		t.Errorf("InstalledVersion for missing binary = %q, want empty", v)
	}

	// A stamped binary reports the stamp without probing.
	bin := filepath.Join(config.BinDir(), "mkcert")
	if err := os.WriteFile(bin, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteStamp("mkcert", "v1.4.4"); err != nil {
		t.Fatal(err)
	}
	if v := InstalledVersion("mkcert"); v != "v1.4.4" {
		t.Errorf("InstalledVersion = %q, want v1.4.4", v)
	}

	// A binary newer than its stamp (self-updated) invalidates the stamp and
	// falls back to probing.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(bin+".version", old, old); err != nil {
		t.Fatal(err)
	}
	orig := probeOutput
	probeOutput = func(path string, args ...string) ([]byte, error) { return []byte("v2.0.0\n"), nil }
	defer func() { probeOutput = orig }()
	if v := InstalledVersion("mkcert"); v != "v2.0.0" {
		t.Errorf("InstalledVersion after newer binary = %q, want probed v2.0.0", v)
	}
}

func TestInstalledVersion_ProbeParsesVersionField(t *testing.T) {
	offline(t)
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.BinDir(), "fnm"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := probeOutput
	probeOutput = func(path string, args ...string) ([]byte, error) { return []byte("fnm 1.39.0\n"), nil }
	defer func() { probeOutput = orig }()
	if v := InstalledVersion("fnm"); v != "1.39.0" {
		t.Errorf("InstalledVersion = %q, want 1.39.0", v)
	}
}

func TestStatusAll_FlagsUpdates(t *testing.T) {
	offline(t)
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	pin := embeddedManifest().Tools["mkcert"].Version

	// mkcert matches the pin (v-prefix differences must not count as updates),
	// composer is stamped behind the pin, fnm is absent.
	if err := os.WriteFile(filepath.Join(config.BinDir(), "mkcert"), []byte("b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteStamp("mkcert", strings.TrimPrefix(pin, "v")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.BinDir(), "composer.phar"), []byte("b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteStamp("composer", "0.0.1"); err != nil {
		t.Fatal(err)
	}

	byName := map[string]ToolStatus{}
	for _, s := range StatusAll(context.Background()) {
		byName[s.Name] = s
	}
	if s := byName["mkcert"]; !s.Present || s.UpdateAvailable {
		t.Errorf("mkcert status = %+v, want present with no update", s)
	}
	if s := byName["composer"]; !s.Present || !s.UpdateAvailable || s.Installed != "0.0.1" {
		t.Errorf("composer status = %+v, want present with update available", s)
	}
	if s := byName["fnm"]; s.Present || s.UpdateAvailable {
		t.Errorf("fnm status = %+v, want absent without update", s)
	}
}

func TestLoad_FetchFailureFallsBackToEmbedded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("LERD_TOOLS_URL", srv.URL)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	m := Load(context.Background())
	if len(m.Tools) != len(embeddedManifest().Tools) {
		t.Errorf("fallback manifest has %d tools, want %d", len(m.Tools), len(embeddedManifest().Tools))
	}
}
