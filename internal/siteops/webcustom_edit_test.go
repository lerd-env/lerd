package siteops

import (
	"os"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/webcustom"
)

func webEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// The web scope edits the file lerd's own web server reads, through the same
// service as the nginx overrides: a good save lands, a bad one is rolled back
// with the reason, and neither reloads anything since the server rereads it.
func TestWebScope_saveValidatesAgainstWhatTheServerReads(t *testing.T) {
	webEnv(t)
	scope, err := ParseNginxScope("web")
	if err != nil {
		t.Fatal(err)
	}
	res, err := SaveCustomNginx("app.test", scope, "headers:\n  X-Frame-Options: DENY\n", false)
	if err != nil || !res.OK {
		t.Fatalf("good save refused: %+v %v", res, err)
	}
	if c, err := webcustom.For("app.test"); err != nil || c.Headers["X-Frame-Options"] != "DENY" {
		t.Fatalf("server reads %+v %v", c, err)
	}

	res, _ = SaveCustomNginx("app.test", scope, "header:\n  X: y\n", false)
	if res.OK || !strings.Contains(res.ValidationOutput, "app.test.yaml") {
		t.Fatalf("bad save accepted or unexplained: %+v", res)
	}
	if c, _ := webcustom.For("app.test"); c.Headers["X-Frame-Options"] != "DENY" {
		t.Error("the bad save was not rolled back")
	}
}

func TestWebScope_readSeedsAnInertTemplate(t *testing.T) {
	webEnv(t)
	c, err := ReadCustomNginx("app.test", NginxWebScope)
	if err != nil || c.Exists || c.Body == "" {
		t.Fatalf("%+v %v", c, err)
	}
	if err := webcustom.Validate("template", []byte(c.Body)); err != nil {
		t.Errorf("the seeded template does not validate: %v", err)
	}
}

// A worktree starts from its site's file, a rename carries it, and removing
// the worktree removes it, the same as the nginx overrides.
func TestWebScope_followsWorktreesAndRenames(t *testing.T) {
	webEnv(t)
	if res, _ := SaveCustomNginx("app.test", NginxWebScope, "headers:\n  X-A: b\n", true); !res.OK {
		t.Fatalf("%+v", res)
	}
	if err := InheritCustomNginxConfig("app.test", "feat.app.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(webcustom.SitePath("feat.app.test")); err != nil {
		t.Fatalf("worktree did not inherit: %v", err)
	}
	if err := MoveCustomNginxConfig("app.test", "shop.test"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{webcustom.SitePath("shop.test"), webcustom.SitePath("feat.shop.test")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("rename lost %s: %v", p, err)
		}
	}
	if err := RemoveCustomNginxConfig("feat.shop.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(webcustom.SitePath("feat.shop.test")); !os.IsNotExist(err) {
		t.Error("removing the worktree left its file")
	}
}

// Resetting the web scope deletes the file and reloads nothing.
func TestWebScope_resetRemovesTheFile(t *testing.T) {
	webEnv(t)
	reloaded := false
	prev := NginxReloadFn
	NginxReloadFn = func() error { reloaded = true; return nil }
	t.Cleanup(func() { NginxReloadFn = prev })
	SaveCustomNginx("app.test", NginxWebScope, "headers:\n  X-A: b\n", false)
	if err := ResetCustomNginx("app.test", NginxWebScope); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(webcustom.SitePath("app.test")); !os.IsNotExist(err) || reloaded {
		t.Fatalf("file still there (%v) or nginx reloaded (%v)", err, reloaded)
	}
}

// The global file applies to every site and is edited the same way.
func TestGlobalWeb_saveValidatesAndResets(t *testing.T) {
	webEnv(t)
	if c, err := ReadGlobalWeb(); err != nil || c.Exists || webcustom.Validate("t", []byte(c.Body)) != nil {
		t.Fatalf("seed %+v %v", c, err)
	}
	if res, err := SaveGlobalWeb("max_body_size: 50m\n", false); err != nil || !res.OK {
		t.Fatalf("%+v %v", res, err)
	}
	if c, _ := webcustom.For("any.test"); !c.MaxBodySet || c.MaxBodyBytes != 50<<20 {
		t.Errorf("not applied to every site: %+v", c)
	}
	if res, _ := SaveGlobalWeb("max_body_size: lots\n", false); res.OK {
		t.Error("an unservable global file was saved")
	}
	if err := ResetGlobalWeb(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(webcustom.GlobalPath()); !os.IsNotExist(err) {
		t.Error("reset left the global file")
	}
}
