package ui

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/webcustom"
)

// The global settings for lerd's own web server are read and saved over the
// same shapes as the global nginx editor, so the dashboard reuses its editor.
func TestWebGlobal_readSaveRefuseReset(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	rec := httptest.NewRecorder()
	handleWebRoutes(rec, httptest.NewRequest("GET", "/api/web/config", nil))
	var read NginxConfigReadResponse
	json.Unmarshal(rec.Body.Bytes(), &read)
	if read.Exists || read.Path != webcustom.GlobalPath() || read.Content == "" {
		t.Fatalf("read %+v", read)
	}

	save := func(body string) NginxConfigWriteResponse {
		rec := httptest.NewRecorder()
		handleWebRoutes(rec, httptest.NewRequest("POST", "/api/web/config", strings.NewReader(body)))
		var res NginxConfigWriteResponse
		json.Unmarshal(rec.Body.Bytes(), &res)
		return res
	}
	if res := save(`{"content":"headers:\n  X-A: b\n"}`); !res.OK {
		t.Fatalf("good save refused: %+v", res)
	}
	if res := save(`{"content":"nope: 1\n"}`); res.OK || !strings.Contains(res.ValidationOutput, "global.yaml") {
		t.Fatalf("bad save: %+v", res)
	}

	rec = httptest.NewRecorder()
	handleWebRoutes(rec, httptest.NewRequest("POST", "/api/web/reset", nil))
	if c, _ := webcustom.For("x.test"); c.Headers["X-A"] != "" {
		t.Errorf("reset kept the header: %+v", c)
	}
	rec = httptest.NewRecorder()
	handleWebRoutes(rec, httptest.NewRequest("POST", "/api/web/restore", strings.NewReader(`{"name":"../../etc/passwd"}`)))
	if !strings.Contains(rec.Body.String(), "invalid backup name") {
		t.Errorf("restore accepted a path: %s", rec.Body.String())
	}
}
