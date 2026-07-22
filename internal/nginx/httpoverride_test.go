package nginx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHTTPOverride seeds the user http-level override file.
func writeHTTPOverride(t *testing.T, tmp, body string) {
	t.Helper()
	dir := filepath.Join(tmp, "lerd", "nginx", "http.d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir http.d: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zz-lerd-user.conf"), []byte(body), 0644); err != nil {
		t.Fatalf("write override: %v", err)
	}
}

func TestHTTPOverrideNames(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	writeHTTPOverride(t, tmp, `# a comment
client_max_body_size 100m;
  gzip on;   # trailing comment
# sendfile off;

map $http_referer $foo {
    default          0;
    keepalive_timeout 9;
}
`)
	got := httpOverrideNames()
	for _, want := range []string{"client_max_body_size", "gzip", "map"} {
		if !got[want] {
			t.Errorf("expected %q in override names, got %v", want, got)
		}
	}
	for _, unwanted := range []string{"sendfile", "keepalive_timeout", "default", "#"} {
		if got[unwanted] {
			t.Errorf("did not expect %q in override names, got %v", unwanted, got)
		}
	}
}

func TestHTTPOverrideNames_missingDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := httpOverrideNames(); len(got) != 0 {
		t.Errorf("expected no names without an override file, got %v", got)
	}
}

// A user override of a directive lerd already sets in http{} must remove
// lerd's default: nginx rejects a duplicate simple directive in the same
// context instead of letting the later one win (issue #1066).
func TestEnsureNginxConfig_dropsOverriddenDefaults(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	writeHTTPOverride(t, tmp, "client_max_body_size 100m;\n")
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	body := readRenderedConf(t, tmp)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "client_max_body_size ") {
			t.Fatalf("lerd default still active, nginx would reject it as duplicate:\n%s", body)
		}
	}
	if !strings.Contains(body, "# client_max_body_size 0;") {
		t.Errorf("expected the dropped default to stay visible as a comment, got:\n%s", body)
	}
	if !strings.Contains(body, "include /etc/nginx/http.d/*.conf;") {
		t.Errorf("http.d include must survive the filter, got:\n%s", body)
	}
	if !strings.Contains(body, "sendfile on;") {
		t.Errorf("untouched defaults must survive the filter, got:\n%s", body)
	}
}

// log_format and access_log may repeat in the same context, so a user
// declaring either must not retire lerd's own: nginx then fails to start
// with "unknown log format lerd_access" and every site goes down.
func TestEnsureNginxConfig_keepsRepeatableLogDirectives(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	writeHTTPOverride(t, tmp, "log_format main '$remote_addr $status';\naccess_log /var/log/nginx/access.log main;\n")
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	body := readRenderedConf(t, tmp)
	if !hasActiveDirective(body, "log_format lerd_access ") {
		t.Errorf("lerd's log_format must survive a user log_format, got:\n%s", body)
	}
	if !hasActiveDirective(body, "access_log syslog:") {
		t.Errorf("lerd's access_log feeds idle-suspend and request stats, got:\n%s", body)
	}
}

// hasActiveDirective reports whether the conf carries prefix as a live
// directive rather than one the filter commented out.
func hasActiveDirective(conf, prefix string) bool {
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}

// The filter only applies to lerd's own http{} defaults. A user directive that
// happens to share a name with something in another block (events{}, or the
// nested location blocks of a vhost) must not disturb it.
func TestEnsureNginxConfig_onlyFiltersHTTPLevel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	writeHTTPOverride(t, tmp, "worker_connections 4096;\n")
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	if body := readRenderedConf(t, tmp); !strings.Contains(body, "worker_connections 1024;") {
		t.Errorf("events{} directive must not be filtered, got:\n%s", body)
	}
}

// Removing the override (the Reset flow) must bring lerd's defaults back.
func TestEnsureNginxConfig_restoresDefaultsAfterReset(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	writeHTTPOverride(t, tmp, "client_max_body_size 100m;\n")
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	if err := os.Remove(filepath.Join(tmp, "lerd", "nginx", "http.d", "zz-lerd-user.conf")); err != nil {
		t.Fatalf("remove override: %v", err)
	}
	if err := EnsureNginxConfig(); err != nil {
		t.Fatalf("EnsureNginxConfig: %v", err)
	}
	if body := readRenderedConf(t, tmp); !strings.Contains(body, "client_max_body_size 0;") {
		t.Errorf("expected the default back after reset, got:\n%s", body)
	}
}

func readRenderedConf(t *testing.T, tmp string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(tmp, "lerd", "nginx", "nginx.conf"))
	if err != nil {
		t.Fatalf("read rendered nginx.conf: %v", err)
	}
	return string(body)
}
