package ui

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

func TestDebugRouteForContext_ResolvesNameToDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{
		Name:    "rapids",
		Domains: []string{"harborlist.test"},
		Path:    t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if got := debugRouteForContext(dumps.Context{Site: "rapids"}, dumps.KindDump); got != "#sites/harborlist.test/dumps/dumps" {
		t.Errorf("route = %q", got)
	}
}

// An event that reaches the notifier without LERD_SITE still carries the
// request domain, which is enough to land on the right site's Debug tab.
func TestDebugRouteForContext_ResolvesSiteFromDomain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{
		Name:    "rapids",
		Domains: []string{"harborlist.test", "admin.harborlist.test"},
		Path:    t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if got := debugRouteForContext(dumps.Context{Domain: "admin.harborlist.test"}, dumps.KindDump); got != "#sites/harborlist.test/dumps/dumps" {
		t.Errorf("route = %q", got)
	}
}

func TestDebugRouteForContext_FallsBackToSitesList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if got := debugRouteForContext(dumps.Context{Type: "cli"}, dumps.KindDump); got != "#sites" {
		t.Errorf("route = %q, want #sites", got)
	}
	if got := debugRouteForContext(dumps.Context{Domain: "gone.test"}, dumps.KindDump); got != "#sites" {
		t.Errorf("unregistered domain route = %q, want #sites", got)
	}
}

func TestNotificationForDump_NoSiteFallsBackToSitesList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	n := notificationForDump(dumps.Event{ID: "z", Kind: "dump", Ctx: dumps.Context{Type: "cli"}})
	if n.URL != "#sites" {
		t.Errorf("URL = %q, want #sites", n.URL)
	}
}

func TestNotificationForNPlusOne_RoutesToSiteDebugTab(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{
		Name:    "rapids",
		Domains: []string{"harborlist.test"},
		Path:    t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	n := notificationForNPlusOne(dumps.Event{Ctx: dumps.Context{Site: "rapids", Request: "GET /users"}}, 4)
	if n.URL != "#sites/harborlist.test/dumps/queries" {
		t.Errorf("URL = %q", n.URL)
	}
}

// Without a site the N+1 warning used to land on the global bridge view, which
// says nothing about the event that was clicked (#1005).
func TestNotificationForNPlusOne_NoSiteFallsBackToSitesList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	n := notificationForNPlusOne(dumps.Event{Ctx: dumps.Context{Worker: "queue:work"}}, 4)
	if n.URL != "#sites" {
		t.Errorf("URL = %q, want #sites", n.URL)
	}
}

// TestDebugRouteNamesTheLens is the click that sent the report: a message
// notification opened the site's Debug tab on whichever lens had last been
// looked at, which showed nothing of what was clicked.
func TestDebugRouteNamesTheLens(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.AddSite(config.Site{
		Name:    "rapids",
		Domains: []string{"harborlist.test"},
		Path:    t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		dumps.KindMessage:   "#sites/harborlist.test/dumps/messages",
		dumps.KindException: "#sites/harborlist.test/dumps/exceptions",
		dumps.KindLog:       "#sites/harborlist.test/dumps/logs",
		dumps.KindJob:       "#sites/harborlist.test/dumps/jobs",
		dumps.KindQuery:     "#sites/harborlist.test/dumps/queries",
	}
	for kind, want := range cases {
		if got := debugRouteForContext(dumps.Context{Site: "rapids"}, kind); got != want {
			t.Errorf("%s route = %q, want %q", kind, got, want)
		}
	}
	// A kind with no lens of its own leaves the tab as the user left it.
	if got := debugRouteForContext(dumps.Context{Site: "rapids"}, "test"); got != "#sites/harborlist.test/dumps" {
		t.Errorf("unknown kind = %q, want the tab without a lens", got)
	}
}
