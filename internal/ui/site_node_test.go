package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	nodeDet "github.com/geodro/lerd/internal/node"
)

// .lerd.yaml node_version outranks .node-version, so switching Node from the
// site page must move both or the old pin keeps winning.
func TestHandleSiteAction_node_updatesLerdYamlPin(t *testing.T) {
	sitePath := setupSecuredSite(t, "nodepin")
	if err := config.SaveProjectConfig(sitePath, &config.ProjectConfig{NodeVersion: "22"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sites/nodepin.test/node?version=24", nil)
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("expected ok response, got %s", rec.Body.String())
	}

	if got, _ := os.ReadFile(filepath.Join(sitePath, ".node-version")); strings.TrimSpace(string(got)) != "24" {
		t.Errorf(".node-version = %q, want 24", got)
	}
	if got, _ := nodeDet.DetectVersion(sitePath); got != "24" {
		t.Errorf("detected Node = %q, want 24", got)
	}
}
