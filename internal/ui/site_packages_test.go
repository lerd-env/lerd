package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// packagesSite registers a site on a user-defined framework that suggests
// lerd/debug, with a composer.json holding the given require-dev section.
func packagesSite(t *testing.T, requireDev string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	fw := "name: acmefw\nlabel: Acme\npublic_dir: public\nsuggest_packages:\n  - name: lerd/debug\n    dev: true\n    reason: Your own timeline rows\n"
	if err := os.MkdirAll(config.FrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.FrameworksDir(), "acmefw.yaml"), []byte(fw), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require-dev":{`+requireDev+`}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "acme", Path: dir, Domains: []string{"acme.test"}, Framework: "acmefw"}); err != nil {
		t.Fatal(err)
	}
}

func packagesCall(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handleSiteAction(rec, req)
	return rec
}

func pendingNames(t *testing.T) []string {
	t.Helper()
	var resp struct {
		Packages []config.PackageSuggestion `json:"packages"`
	}
	if err := json.Unmarshal(packagesCall(t, http.MethodGet, "/api/sites/acme.test/packages").Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range resp.Packages {
		names = append(names, p.Name)
	}
	return names
}

func TestPackagesRoute_offersOnlyWhatIsMissing(t *testing.T) {
	packagesSite(t, "")
	if got := pendingNames(t); len(got) != 1 || got[0] != "lerd/debug" {
		t.Fatalf("pending = %v, want lerd/debug", got)
	}

	packagesSite(t, `"lerd/debug":"^0.1"`)
	if got := pendingNames(t); len(got) != 0 {
		t.Fatalf("pending = %v, want nothing once required", got)
	}
}

func TestPackagesRoute_dismissStopsTheOffer(t *testing.T) {
	packagesSite(t, "")
	if rec := packagesCall(t, http.MethodPost, "/api/sites/acme.test/packages/dismiss?name=lerd/debug"); !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("dismiss = %s", rec.Body.String())
	}
	if got := pendingNames(t); len(got) != 0 {
		t.Fatalf("pending = %v, want nothing after dismissing", got)
	}
}

func TestPackagesRoute_installRefusesWhatIsNotOffered(t *testing.T) {
	packagesSite(t, "")
	rec := packagesCall(t, http.MethodPost, "/api/sites/acme.test/packages/install?name=evil/pkg")
	if !strings.Contains(rec.Body.String(), "not a package suggested") {
		t.Fatalf("install = %s", rec.Body.String())
	}
}
