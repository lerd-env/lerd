package goserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// phpFixture's PHP-FPM stand-in reports the script, path info, branch and one
// framework parameter, and sleeps when asked to.
func phpFixture(t *testing.T) (*Handler, config.Site) {
	t.Helper()
	h, _ := fixture(t)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { ln.Close() })
	go fcgi.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env := fcgi.ProcessEnv(r)
		if r.URL.Path == "/slow" {
			time.Sleep(3 * time.Second)
		}
		fmt.Fprintf(w, "script=%s|path_info=%s|branch=%s|tb=%s", env["DOCUMENT_URI"], env["PATH_INFO"], env["LERD_BRANCH"], env["TESTBENCH_WORKING_PATH"])
	}))
	h.Upstream = func(config.Site) string { return ln.Addr().String() }
	return h, h.Sites()[0]
}

func writeProject(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

// With front-controller routing every PHP path runs index.php, and a path
// under it carries the rest as PATH_INFO.
func TestPHP_frontControllerRoutesEveryScriptThroughIndex(t *testing.T) {
	h, site := phpFixture(t)
	writeProject(t, site.Path, "front_controller: true\n")
	for url, want := range map[string]string{
		"http://app.test/info.php":          "script=/index.php|",
		"http://app.test/index.php/a/b?x=1": "script=/index.php|",
	} {
		if got := get(h, "GET", url, "").Body.String(); !strings.HasPrefix(got, want) {
			t.Errorf("%s: %q, want prefix %q", url, got, want)
		}
	}
}

// Without it a PHP path is PHP-FPM's to answer, a missing one included, as
// nginx's \.php$ location hands it over rather than the front controller.
func TestPHP_missingScriptGoesToPHPNotTheFrontController(t *testing.T) {
	h, _ := phpFixture(t)
	if got := get(h, "GET", "http://app.test/nope.php", "").Body.String(); !strings.HasPrefix(got, "script=/nope.php|") {
		t.Fatalf("%q", got)
	}
}

func TestPHP_frameworkParamsReachPHP(t *testing.T) {
	h, site := phpFixture(t)
	if err := config.SaveFramework(&config.Framework{Name: "tb", Label: "Testbench", Nginx: &config.FrameworkNginx{
		FastCGIParams: map[string]string{"TESTBENCH_WORKING_PATH": "{{root}}", "BAD": "$x"},
	}}); err != nil {
		t.Fatal(err)
	}
	site.Framework = "tb"
	h.Sites = func() []config.Site { return []config.Site{site} }
	got := get(h, "GET", "http://app.test/", "").Body.String()
	if want := "tb=" + site.Path; !strings.HasSuffix(got, want) {
		t.Fatalf("%q, want suffix %q", got, want)
	}
}

// PHP slower than the site's request timeout answers 504, as nginx's
// fastcgi_read_timeout does, rather than holding the browser for ever.
func TestPHP_slowerThanTheTimeoutIs504(t *testing.T) {
	h, site := phpFixture(t)
	writeProject(t, site.Path, "request_timeout: 1\n")
	if rec := get(h, "GET", "http://app.test/slow", ""); rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status %d, want 504", rec.Code)
	}
}

// Go's FastCGI responder swallows PATH_INFO, so the split is checked on the
// routing itself: the part after /index.php travels as PATH_INFO.
func TestPhpScript_frontControllerSplitsPathInfo(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		path               string
		fc                 bool
		script, info, file string
	}{
		{"/index.php/a/b", true, "/index.php", "/a/b", ""},
		{"/index.php", true, "/index.php", "", ""},
		{"/index.phpx", true, "/index.php", "", ""},
		{"/other.php", true, "/index.php", "", ""},
		{"/other.php", false, "/other.php", "", ""},
		{"/route", false, "/index.php", "", ""},
	}
	for _, c := range cases {
		script, info, file := phpScript(root, c.path, c.fc)
		if script != c.script || info != c.info || file != c.file {
			t.Errorf("%s fc=%v: got (%q %q %q), want (%q %q %q)", c.path, c.fc, script, info, file, c.script, c.info, c.file)
		}
	}
}
