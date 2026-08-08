package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

func queryEventFor(site, request string) dumps.Event {
	data, _ := json.Marshal(dumps.QueryData{SQL: "SELECT * FROM cachetags WHERE tag = 'x'"})
	return dumps.Event{
		V:    1,
		Kind: dumps.KindQuery,
		Data: data,
		Ctx:  dumps.Context{RID: "r1", Site: site, Request: request},
	}
}

// registerFrameworkSite installs a definition and a site on it, in a sandboxed
// config so the developer's own install is never read.
func registerFrameworkSite(t *testing.T, name, defYAML string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	store := config.StoreFrameworksDir()
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, name+".yaml"), []byte(defYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := config.AddSite(config.Site{Name: "s", Domains: []string{"s.test"}, Path: dir, Framework: name}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A content management system issues the repeats from its own entity, config
// and cache layers, so the warning names a loop inside the framework that
// nobody using it can change. Browsing such a site flooded the desktop.
func TestNPlusOne_frameworkCanDeclineTheWarning(t *testing.T) {
	registerFrameworkSite(t, "cmsish", "name: cmsish\nlabel: CMSish\npublic_dir: web\nnotifications:\n  nplusone: false\n")

	tracker := newNPlusOneTracker()
	for range 10 {
		if n := tracker.observe(queryEventFor("s", "GET /admin/content")); n != nil {
			t.Fatalf("a framework that declined the warning produced one: %s", n.Body)
		}
	}
}

// A framework that says nothing keeps the warning, which is most of them: the
// queries come from the code the developer wrote.
func TestNPlusOne_frameworkWithoutADeclarationStillWarns(t *testing.T) {
	registerFrameworkSite(t, "appish", "name: appish\nlabel: Appish\npublic_dir: public\n")

	tracker := newNPlusOneTracker()
	var got bool
	for range 5 {
		if n := tracker.observe(queryEventFor("s", "GET /orders")); n != nil {
			got = true
		}
	}
	if !got {
		t.Error("a framework with no declaration stopped warning")
	}
}

func TestFrameworkWarnsNPlusOne_defaults(t *testing.T) {
	off := false
	on := true
	if (&config.Framework{}).WarnsNPlusOne() != true {
		t.Error("a framework declaring nothing should warn")
	}
	if (&config.Framework{Notifications: &config.FrameworkNotifications{NPlusOne: &off}}).WarnsNPlusOne() {
		t.Error("a framework declining should not warn")
	}
	if !(&config.Framework{Notifications: &config.FrameworkNotifications{NPlusOne: &on}}).WarnsNPlusOne() {
		t.Error("a framework declaring true should warn")
	}
	var nilFW *config.Framework
	if !nilFW.WarnsNPlusOne() {
		t.Error("no framework at all should warn")
	}
}
