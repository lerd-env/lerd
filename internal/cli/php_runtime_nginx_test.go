package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// stubWebServerMove records what moving nginx asks for, in order, so no test
// rewrites or restarts the developer's own nginx.
func stubWebServerMove(t *testing.T, nowOnHost bool) *[]string {
	t.Helper()
	var calls []string
	prevOn, prevRest, prevUnit, prevRestart, prevImage, prevHosts := moveOnHostFn, moveRewriteRestFn, moveRewriteUnitFn, moveRestartUnitFn, moveEnsureImageFn, moveContainerHostsFn
	t.Cleanup(func() {
		moveOnHostFn, moveRewriteRestFn, moveRewriteUnitFn, moveRestartUnitFn, moveEnsureImageFn, moveContainerHostsFn = prevOn, prevRest, prevUnit, prevRestart, prevImage, prevHosts
	})
	moveContainerHostsFn = func() error { calls = append(calls, "hosts"); return nil }
	moveOnHostFn = func() bool { return nowOnHost }
	moveRewriteRestFn = func([]config.Site) { calls = append(calls, "configs") }
	moveRewriteUnitFn = func() (bool, error) { calls = append(calls, "unit"); return true, nil }
	moveRestartUnitFn = func() error { calls = append(calls, "restart"); return nil }
	moveEnsureImageFn = func() { calls = append(calls, "image") }
	return &calls
}

// Switching PHP to the host takes nginx with it: every config is rewritten for
// the host, the unit swapped, and nginx restarted on it rather than reloaded.
func TestMoveWebServer_followsPHPToTheHost(t *testing.T) {
	calls := stubWebServerMove(t, true)
	if !moveWebServer(true, nil) {
		t.Fatal("nginx did not move with PHP")
	}
	if got := strings.Join(*calls, ","); got != "configs,unit,restart,hosts" {
		t.Errorf("move = %s, want configs,unit,restart,hosts", got)
	}
}

// Coming back, the container needs its image before it can start: moving to
// the host offers to remove images, and nginx's may be among them.
func TestMoveWebServer_bringsTheContainerBack(t *testing.T) {
	calls := stubWebServerMove(t, false)
	if !moveWebServer(true, nil) {
		t.Fatal("nginx did not move back")
	}
	if got := strings.Join(*calls, ","); got != "configs,image,unit,restart,hosts" {
		t.Errorf("move = %s, want configs,image,unit,restart,hosts", got)
	}
}

func TestMoveWebServer_staysPutWhenItsHomeDidNotChange(t *testing.T) {
	calls := stubWebServerMove(t, true)
	if moveWebServer(false, nil) || len(*calls) != 0 {
		t.Errorf("moved without a change of home: %v", *calls)
	}
}

// The switch skips paused sites and host proxies, so their worktrees are
// rewritten with them: one left naming /etc/nginx stops the host's nginx.
func TestRewriteUnmovedWorktrees_followsTheSitesKind(t *testing.T) {
	var got []string
	prevList, prevPaused, prevProxy := unmovedWorktreesFn, unmovedPausedWorktreeFn, unmovedHostProxyWorktreeFn
	t.Cleanup(func() {
		unmovedWorktreesFn, unmovedPausedWorktreeFn, unmovedHostProxyWorktreeFn = prevList, prevPaused, prevProxy
	})
	unmovedWorktreesFn = func(path, domain string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Path: path + "/wt", Domain: "feat." + domain}}, nil
	}
	unmovedPausedWorktreeFn = func(domain, certDomain, _ string, secured bool) error {
		got = append(got, "paused "+domain)
		return nil
	}
	unmovedHostProxyWorktreeFn = func(_ config.Site, _, domain string, _ bool) error {
		got = append(got, "proxy "+domain)
		return nil
	}
	rewriteUnmovedWorktrees(config.Site{Name: "a", Domains: []string{"a.test"}, Path: "/a", Paused: true})
	rewriteUnmovedWorktrees(config.Site{Name: "b", Domains: []string{"b.test"}, Path: "/b", HostPort: 3000})
	if want := "paused feat.a.test,proxy feat.b.test"; strings.Join(got, ",") != want {
		t.Errorf("rewrote %v, want %s", got, want)
	}
}

// install's own loop rewrites only the sites, so when it moves nginx their
// worktrees follow too, beside everything the runtime switch skips.
func TestFollowNginxMove_rewritesActiveWorktrees(t *testing.T) {
	var got []string
	prevList, prevWrite, prevRest := switchWorktreesFor, switchWriteWorktreeVhost, moveRewriteRestFn
	t.Cleanup(func() {
		switchWorktreesFor, switchWriteWorktreeVhost, moveRewriteRestFn = prevList, prevWrite, prevRest
	})
	switchWorktreesFor = func(path, domain string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Path: path + "/wt", Domain: "feat." + domain}}, nil
	}
	switchWriteWorktreeVhost = func(domain, _, _, _, _, _ string, _ bool) error {
		got = append(got, domain)
		return nil
	}
	moveRewriteRestFn = func([]config.Site) { got = append(got, "rest") }
	followNginxMove([]config.Site{
		{Name: "a", Domains: []string{"a.test"}, Path: "/a"},
		{Name: "p", Domains: []string{"p.test"}, Path: "/p", Paused: true},
	})
	if want := "feat.a.test,rest"; strings.Join(got, ",") != want {
		t.Errorf("rewrote %v, want %s", got, want)
	}
}

// A FrankenPHP site is not moved by the switch, so the sweep of what it leaves
// rewrites its worktrees, or a secured one keeps the container's certificate paths.
func TestRewriteUnmovedConfigs_rewritesFrankenPHPWorktrees(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var got []string
	prevList, prevWrite, prevOnHost := switchWorktreesFor, switchWriteWorktreeVhost, moveOnHostFn
	t.Cleanup(func() { switchWorktreesFor, switchWriteWorktreeVhost, moveOnHostFn = prevList, prevWrite, prevOnHost })
	moveOnHostFn = func() bool { return false }
	switchWorktreesFor = func(path, domain string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Path: path + "/wt", Domain: "feat." + domain}}, nil
	}
	switchWriteWorktreeVhost = func(domain, _, _, _, _, _ string, _ bool) error {
		got = append(got, domain)
		return nil
	}
	rewriteUnmovedConfigs([]config.Site{{Name: "fp", Domains: []string{"fp.test"}, Path: t.TempDir(), Runtime: "frankenphp"}})
	if want := "feat.fp.test"; strings.Join(got, ",") != want {
		t.Errorf("rewrote %v, want %s", got, want)
	}
}

// A worktree that pins its own PHP version keeps it when the sweep rewrites it.
func TestRegenerateWorktreeVhosts_keepsTheWorktreesPHP(t *testing.T) {
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".lerd.yaml"), []byte("php_version: \"8.3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []string
	prevList, prevWrite := switchWorktreesFor, switchWriteWorktreeVhost
	t.Cleanup(func() { switchWorktreesFor, switchWriteWorktreeVhost = prevList, prevWrite })
	switchWorktreesFor = func(_, domain string) ([]gitpkg.Worktree, error) {
		return []gitpkg.Worktree{{Path: wt, Domain: "feat." + domain}}, nil
	}
	switchWriteWorktreeVhost = func(domain, _, php, _, _, _ string, _ bool) error {
		got = append(got, domain+"@"+php)
		return nil
	}
	regenerateWorktreeVhosts(&config.Site{Name: "fp", Domains: []string{"fp.test"}, Path: t.TempDir(), PHPVersion: "8.5", Runtime: "frankenphp"})
	if want := "feat.fp.test@8.3"; strings.Join(got, ",") != want {
		t.Errorf("wrote %v, want %s", got, want)
	}
}

// The host's nginx binds unforwarded ports by LAN exposure in its configs, so a
// toggle rewrites and reloads them; container nginx takes it from its quadlet.
func TestRewriteHostNginxForLAN(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.SaveSites(&config.SiteRegistry{}); err != nil {
		t.Fatal(err)
	}
	prevReload := lanReloadFn
	t.Cleanup(func() { lanReloadFn = prevReload })
	for _, onHost := range []bool{true, false} {
		calls := stubWebServerMove(t, onHost)
		lanReloadFn = func() error { *calls = append(*calls, "reload"); return nil }
		if err := rewriteHostNginxForLAN(); err != nil {
			t.Fatal(err)
		}
		want := map[bool]string{true: "configs,reload", false: ""}[onHost]
		if got := strings.Join(*calls, ","); got != want {
			t.Errorf("on host %v: %s, want %q", onHost, got, want)
		}
	}
}
