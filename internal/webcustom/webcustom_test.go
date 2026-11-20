package webcustom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestFor_mergesGlobalThenSite(t *testing.T) {
	setup(t)
	write(t, GlobalPath(), `
headers:
  X-Frame-Options: DENY
  X-Global: "1"
max_body_size: 10M
`)
	write(t, SitePath("app.test"), `
headers:
  X-Frame-Options: SAMEORIGIN
redirects:
  - from: /old
    to: /new
    status: 301
proxies:
  - path: /api
    to: http://host.containers.internal:9000
roots:
  - path: /setup
    root: "{{root}}"
max_body_size: 0
fastcgi_params:
  APP_FLAG: "on"
`)
	c, err := For("app.test")
	if err != nil {
		t.Fatal(err)
	}
	if c.Headers["X-Frame-Options"] != "SAMEORIGIN" || c.Headers["X-Global"] != "1" {
		t.Errorf("headers %v: the site's own wins, the global ones stay", c.Headers)
	}
	if len(c.Redirects) != 1 || c.Redirects[0].Status != 301 || len(c.Proxies) != 1 || len(c.Roots) != 1 {
		t.Errorf("rules %+v", c)
	}
	if c.MaxBodyBytes != 0 || !c.MaxBodySet {
		t.Errorf("an explicit 0 lifts the global cap: %d set=%v", c.MaxBodyBytes, c.MaxBodySet)
	}
	if c.FastCGIParams["APP_FLAG"] != "on" {
		t.Errorf("params %v", c.FastCGIParams)
	}
}

func TestFor_noFilesIsEmpty(t *testing.T) {
	setup(t)
	c, err := For("app.test")
	if err != nil || len(c.Headers) != 0 || c.MaxBodySet {
		t.Fatalf("%+v %v", c, err)
	}
}

// A typo is refused by name rather than silently doing nothing, and a value
// that cannot work is refused before it is ever served.
func TestFor_refusesWhatItCannotServe(t *testing.T) {
	cases := map[string]string{
		"unknown key":        "header:\n  X: y\n",
		"redirect status":    "redirects:\n  - from: /a\n    to: /b\n    status: 200\n",
		"redirect from":      "redirects:\n  - from: a\n    to: /b\n",
		"proxy scheme":       "proxies:\n  - path: /api\n    to: ftp://x\n",
		"proxy path":         "proxies:\n  - path: api\n    to: http://x\n",
		"root placeholder":   "roots:\n  - path: /setup\n    root: \"{{nope}}\"\n",
		"header name":        "headers:\n  \"Bad Header\": x\n",
		"header value":       "headers:\n  X-A: \"a\\nb\"\n",
		"body size":          "max_body_size: lots\n",
		"fastcgi param name": "fastcgi_params:\n  \"1BAD\": x\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			setup(t)
			write(t, SitePath("app.test"), body)
			if _, err := For("app.test"); err == nil {
				t.Fatalf("accepted %q", body)
			} else if !strings.Contains(err.Error(), "app.test.yaml") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestExpandRoot(t *testing.T) {
	if got := ExpandRoot("{{root}}/x", "/srv/app", "/srv/app/pub"); got != "/srv/app/x" {
		t.Errorf("root %q", got)
	}
	if got := ExpandRoot("{{public}}", "/srv/app", "/srv/app/pub"); got != "/srv/app/pub" {
		t.Errorf("public %q", got)
	}
}
