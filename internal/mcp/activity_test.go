package mcp

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestSiteForToolArgs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	if err := config.AddSite(config.Site{Name: "myapp", Path: "/srv/myapp", PHPVersion: "8.4"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"explicit site name", map[string]any{"site": "myapp"}, "myapp"},
		{"site beats path", map[string]any{"site": "myapp", "path": "/elsewhere"}, "myapp"},
		{"path resolves to site", map[string]any{"path": "/srv/myapp"}, "myapp"},
		{"unknown path", map[string]any{"path": "/no/such/site"}, ""},
	}
	for _, c := range cases {
		if got := siteForToolArgs(c.args); got != c.want {
			t.Errorf("%s: siteForToolArgs = %q, want %q", c.name, got, c.want)
		}
	}
}

// In an mcp:inject context the tool args carry no explicit site/path; the
// activity ping must still resolve the injected default site so idle-suspend
// doesn't sleep the project the agent is actively working on.
func TestSiteForToolArgs_FallsBackToDefaultSite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	if err := config.AddSite(config.Site{Name: "myapp", Path: "/srv/myapp", PHPVersion: "8.4"}); err != nil {
		t.Fatalf("seed site: %v", err)
	}

	orig := defaultSitePath
	defaultSitePath = "/srv/myapp"
	t.Cleanup(func() { defaultSitePath = orig })

	if got := siteForToolArgs(map[string]any{"action": "list"}); got != "myapp" {
		t.Errorf("siteForToolArgs with no explicit site/path = %q, want myapp (the injected default)", got)
	}
}
