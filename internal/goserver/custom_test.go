package goserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/webcustom"
)

func customFixture(t *testing.T, yaml string) (*Handler, config.Site) {
	t.Helper()
	h, _ := fixture(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := fcgi.ProcessEnv(r)
		fmt.Fprintf(w, "script=%s|flag=%s", env["SCRIPT_FILENAME"], env["APP_FLAG"])
	}))
	h.Upstream = func(config.Site) string { return ln.Addr().String() }
	site := h.Sites()[0]
	if err := os.MkdirAll(filepath.Dir(webcustom.SitePath("app.test")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webcustom.SitePath("app.test"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return h, site
}

func TestCustom_headersOnEveryResponse(t *testing.T) {
	h, _ := customFixture(t, "headers:\n  X-Frame-Options: DENY\n")
	if got := get(h, "GET", "http://app.test/", "").Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options %q", got)
	}
}

func TestCustom_redirectsAPathAndWhatIsUnderIt(t *testing.T) {
	h, _ := customFixture(t, "redirects:\n  - from: /old\n    to: https://example.test/new\n    status: 301\n")
	for _, p := range []string{"/old", "/old/x"} {
		rec := get(h, "GET", "http://app.test"+p, "")
		if rec.Code != 301 || rec.Header().Get("Location") != "https://example.test/new" {
			t.Errorf("%s: %d %q", p, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := get(h, "GET", "http://app.test/older", ""); rec.Code == 301 {
		t.Error("/older shares the prefix but is not under /old")
	}
}

func TestCustom_proxiesAPath(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "api %s %s", r.Host, r.URL.Path)
	}))
	t.Cleanup(api.Close)
	h, _ := customFixture(t, "proxies:\n  - path: /api\n    to: "+api.URL+"\n")
	if got := get(h, "GET", "http://app.test/api/users", "").Body.String(); got != "api app.test /api/users" {
		t.Fatalf("%q", got)
	}
}

// A root serves its path from another directory, PHP included, as Magento's
// /setup is served from the project root rather than pub/.
func TestCustom_rootServesAPathFromAnotherDirectory(t *testing.T) {
	h, site := customFixture(t, "roots:\n  - path: /setup\n    root: \"{{root}}\"\n")
	if err := os.MkdirAll(filepath.Join(site.Path, "setup"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(site.Path, "setup", "index.php"), nil, 0o644)
	real, _ := filepath.EvalSymlinks(site.Path)
	if got := get(h, "GET", "http://app.test/setup/index.php", "").Body.String(); !strings.HasPrefix(got, "script="+real+"/setup/index.php|") {
		t.Fatalf("%q", got)
	}
}

func TestCustom_bodyOverTheCapIs413(t *testing.T) {
	h, _ := customFixture(t, "max_body_size: 1k\n")
	if rec := get(h, "POST", "http://app.test/upload", strings.Repeat("x", 2048)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", rec.Code)
	}
	if rec := get(h, "POST", "http://app.test/upload", "small"); rec.Code != 200 {
		t.Fatalf("small body %d", rec.Code)
	}
}

func TestCustom_fastcgiParamsReachPHP(t *testing.T) {
	h, _ := customFixture(t, "fastcgi_params:\n  APP_FLAG: \"on\"\n")
	if got := get(h, "GET", "http://app.test/", "").Body.String(); !strings.HasSuffix(got, "|flag=on") {
		t.Fatalf("%q", got)
	}
}

// A file that cannot be served answers 500 naming it, rather than the site
// quietly running without the customisation the user wrote.
func TestCustom_brokenFileIs500NamingIt(t *testing.T) {
	h, _ := customFixture(t, "header:\n  X: y\n")
	rec := get(h, "GET", "http://app.test/", "")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "app.test.yaml") {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}

// A framework that needs a path served from elsewhere declares it in its store
// definition, the Go web server's counterpart of an nginx snippet.
func TestCustom_frameworkRootServesAPathFromTheProject(t *testing.T) {
	h, site := customFixture(t, "")
	if err := config.SaveFramework(&config.Framework{Name: "shop", Label: "Shop", Web: &config.FrameworkWeb{
		Roots: []config.FrameworkWebRoot{{Path: "/setup", Root: "{{root}}"}},
	}}); err != nil {
		t.Fatal(err)
	}
	site.Framework = "shop"
	h.Sites = func() []config.Site { return []config.Site{site} }
	os.MkdirAll(filepath.Join(site.Path, "setup"), 0o755)
	os.WriteFile(filepath.Join(site.Path, "setup", "index.php"), nil, 0o644)
	real, _ := filepath.EvalSymlinks(site.Path)
	if got := get(h, "GET", "http://app.test/setup/index.php", "").Body.String(); !strings.HasPrefix(got, "script="+real+"/setup/index.php|") {
		t.Fatalf("%q", got)
	}
}

// A store root outside the project is refused, never served.
func TestCustom_frameworkRootOutsideTheProjectIsRefused(t *testing.T) {
	h, site := customFixture(t, "")
	for _, root := range []string{"/etc", "{{root}}/../..", "{{nope}}"} {
		config.SaveFramework(&config.Framework{Name: "shop", Label: "Shop", Web: &config.FrameworkWeb{
			Roots: []config.FrameworkWebRoot{{Path: "/setup", Root: root}},
		}})
		site.Framework = "shop"
		h.Sites = func() []config.Site { return []config.Site{site} }
		got := get(h, "GET", "http://app.test/setup/index.php", "").Body.String()
		if !strings.Contains(got, "/public/setup/index.php") {
			t.Errorf("root %q was used: %q", root, got)
		}
	}
}
