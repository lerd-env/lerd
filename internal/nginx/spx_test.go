package nginx

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestGenerateVhost_ProfilerOnInjectsSpxEnabled(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, _ := config.LoadGlobal()
	cfg.Profiler.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}

	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp"}
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	if !strings.Contains(content, "SPX_KEY=$spx_key") {
		t.Errorf("expected SPX_KEY cookie injection in:\n%s", content)
	}
	if !strings.Contains(content, "SPX_ENABLED=1") {
		t.Errorf("profiler on should inject SPX_ENABLED=1 in:\n%s", content)
	}
}

func TestGenerateVhost_ProfilerOffOmitsSpxEnabled(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp"}
	if err := GenerateVhost(site, "8.3"); err != nil {
		t.Fatalf("GenerateVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "myapp.test.conf"))
	if !strings.Contains(content, "SPX_KEY=$spx_key") {
		t.Errorf("expected SPX_KEY cookie injection even when off in:\n%s", content)
	}
	if strings.Contains(content, "SPX_ENABLED=1") {
		t.Errorf("profiler off must not inject SPX_ENABLED in:\n%s", content)
	}
}

func TestEnsureForwardedConf_WritesSpxKeyMap(t *testing.T) {
	confD := setupConfD(t)
	if err := EnsureForwardedConf(); err != nil {
		t.Fatalf("EnsureForwardedConf: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "_forwarded.conf"))
	if !strings.Contains(content, "map $http_x_forwarded_host $spx_key") {
		t.Errorf("expected $spx_key map in:\n%s", content)
	}
	key, err := config.LoadOrGenerateProfilerKey()
	if err != nil {
		t.Fatalf("LoadOrGenerateProfilerKey: %v", err)
	}
	if !strings.Contains(content, key) {
		t.Errorf("expected generated key %q in the map", key)
	}
}

func TestEnsureProfilerVhost_WritesVhost(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := EnsureProfilerVhost(); err != nil {
		t.Fatalf("EnsureProfilerVhost: %v", err)
	}
	content := readConf(t, filepath.Join(confD, "_profiler.conf"))
	for _, want := range []string{"server_name profiler.localhost", "SPX_KEY=$spx_key", "fastcgi_pass"} {
		if !strings.Contains(content, want) {
			t.Errorf("profiler vhost missing %q in:\n%s", want, content)
		}
	}
}

// The state marker is how a caller knows nginx has finished picking up a toggle:
// it answers with the setting the serving configuration was generated from, so it
// has to carry the current one and cost no PHP.
func TestEnsureProfilerVhost_CarriesTheProfilerState(t *testing.T) {
	confD := setupConfD(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := EnsureProfilerVhost(); err != nil {
		t.Fatalf("EnsureProfilerVhost: %v", err)
	}
	off := readConf(t, filepath.Join(confD, "_profiler.conf"))
	if !strings.Contains(off, "location = "+ProfilerStatePath) {
		t.Errorf("profiler vhost missing the %s marker in:\n%s", ProfilerStatePath, off)
	}
	if !strings.Contains(off, `return 200 "off"`) {
		t.Errorf("marker should answer off while the profiler is off:\n%s", off)
	}

	cfg, _ := config.LoadGlobal()
	cfg.Profiler.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if err := EnsureProfilerVhost(); err != nil {
		t.Fatalf("EnsureProfilerVhost: %v", err)
	}
	on := readConf(t, filepath.Join(confD, "_profiler.conf"))
	if !strings.Contains(on, `return 200 "on"`) {
		t.Errorf("marker should answer on once the profiler is armed:\n%s", on)
	}
}

func TestServedProfilerState_ReadsWhatNginxAnswers(t *testing.T) {
	for _, tc := range []struct {
		body   string
		wantOn bool
		wantOK bool
	}{
		{"on", true, true},
		{"off", false, true},
		{"", false, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != ProfilerStatePath || r.Host != "profiler.localhost" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(tc.body))
		}))
		host := strings.TrimPrefix(srv.URL, "http://")
		on, ok := servedProfilerStateAt(host)
		if on != tc.wantOn || ok != tc.wantOK {
			t.Errorf("body %q: got (%v, %v), want (%v, %v)", tc.body, on, ok, tc.wantOn, tc.wantOK)
		}
		srv.Close()
	}

	// An install whose vhost predates the marker 404s, and nginx being down is a
	// dial error: both are "cannot tell", never a state.
	if _, ok := servedProfilerStateAt("127.0.0.1:1"); ok {
		t.Error("unreachable nginx should report ok=false, not a state")
	}
}
