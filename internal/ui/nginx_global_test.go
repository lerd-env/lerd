package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/nginx"
)

// setupGlobalNginx prepares an isolated data/config home and pre-writes the
// lerd-nginx quadlet so the handler's RewriteNginxQuadlet call reports no
// change and the save takes the validate + reload path instead of restarting
// a container that does not exist under test.
func setupGlobalNginx(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stubNginxReload(t)
	stubNginxTest(t, "nginx: configuration file /etc/nginx/nginx.conf test is successful", nil)
	if _, err := nginx.RewriteNginxQuadlet(); err != nil {
		t.Fatalf("seed nginx quadlet: %v", err)
	}
	return data
}

func postGlobalNginx(t *testing.T, content string) NginxConfigWriteResponse {
	t.Helper()
	body, _ := json.Marshal(NginxConfigWriteRequest{Content: content})
	req := httptest.NewRequest(http.MethodPost, "/api/nginx/config", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleNginxConfig(rec, req)
	var resp NginxConfigWriteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
	}
	return resp
}

func renderedNginxConf(t *testing.T, data string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(data, "lerd", "nginx", "nginx.conf"))
	if err != nil {
		t.Fatalf("read nginx.conf: %v", err)
	}
	return string(body)
}

// Saving an override for a directive lerd already sets in http{} must leave
// nginx.conf without lerd's own copy, or nginx refuses the config as a
// duplicate directive (issue #1066).
func TestHandleNginxConfig_saveDropsCollidingDefault(t *testing.T) {
	data := setupGlobalNginx(t)
	resp := postGlobalNginx(t, "client_max_body_size 100m;\n")
	if !resp.OK {
		t.Fatalf("save failed: %+v", resp)
	}
	conf := renderedNginxConf(t, data)
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "client_max_body_size ") {
			t.Fatalf("lerd default still active alongside the override:\n%s", conf)
		}
	}
}

// Reset drops the override, so lerd's defaults have to come back with it.
func TestHandleNginxConfigReset_restoresDefault(t *testing.T) {
	data := setupGlobalNginx(t)
	if resp := postGlobalNginx(t, "client_max_body_size 100m;\n"); !resp.OK {
		t.Fatalf("save failed: %+v", resp)
	}
	rec := httptest.NewRecorder()
	handleNginxConfigReset(rec, httptest.NewRequest(http.MethodPost, "/api/nginx/reset", nil))
	var resp NginxConfigResetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
	}
	if !resp.OK {
		t.Fatalf("reset failed: %+v", resp)
	}
	if conf := renderedNginxConf(t, data); !strings.Contains(conf, "client_max_body_size 0;") {
		t.Errorf("expected lerd's default back after reset, got:\n%s", conf)
	}
}
