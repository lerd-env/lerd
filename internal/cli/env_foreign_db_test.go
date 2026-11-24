package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

const dbnginEnv = "APP_KEY=base64:x\nDB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=my_real_data\nDB_USERNAME=root\nDB_PASSWORD=\nREDIS_HOST=127.0.0.1\nREDIS_PORT=6399\n"

// foreignDBSite links a Laravel-shaped project and makes it the working directory.
func foreignDBSite(t *testing.T, env string) (dir string, calls *provisionCalls) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir = t.TempDir()
	for name, body := range map[string]string{"artisan": "", ".env": env} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: dir, Framework: "laravel", HostPort: 3100}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	calls = stubProvisionSeams(t, nil)
	prev := envKeepForeignDB
	t.Cleanup(func() { envKeepForeignDB = prev })
	return dir, calls
}

func envOf(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasDB(calls *provisionCalls) bool {
	return len(calls.databases) > 0 || slices.Contains(calls.started, "mysql")
}

func TestRunEnv_AutomaticRunKeepsAForeignDatabase(t *testing.T) {
	dir, calls := foreignDBSite(t, dbnginEnv)
	envKeepForeignDB = true
	if err := runEnv(nil, nil); err != nil {
		t.Fatal(err)
	}
	got := envOf(t, dir)
	for _, line := range []string{"DB_HOST=127.0.0.1\n", "DB_PORT=3306\n", "DB_DATABASE=my_real_data\n", "DB_USERNAME=root\n", "DB_PASSWORD=\n"} {
		if !strings.Contains(got, line) {
			t.Errorf("env lost %q:\n%s", line, got)
		}
	}
	if hasDB(calls) {
		t.Errorf("touched lerd's mysql: %+v", calls)
	}
	// Everything else still follows.
	if strings.Contains(got, "REDIS_PORT=6399") || !slices.Contains(calls.started, "redis") {
		t.Errorf("redis did not follow:\n%s\n%+v", got, calls)
	}
	site, err := config.FindSite("shop")
	if err != nil || slices.Contains(site.WiredServices, "mysql") || !slices.Contains(site.WiredServices, "redis") {
		t.Errorf("wired = %v, %v", site.WiredServices, err)
	}
}

// Picking mysql in .lerd.yaml is not a reason to move a connection on its own.
func TestRunEnv_AutomaticRunKeepsAForeignDatabaseDespiteAPick(t *testing.T) {
	dir, calls := foreignDBSite(t, dbnginEnv)
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("services:\n  - mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envKeepForeignDB = true
	if err := runEnv(nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(envOf(t, dir), "DB_DATABASE=my_real_data\n") || hasDB(calls) {
		t.Errorf("moved:\n%s\n%+v", envOf(t, dir), calls)
	}
}

func TestRunEnv_AutomaticRunStillMovesLerdsOwnPort(t *testing.T) {
	dir, calls := foreignDBSite(t, "APP_KEY=base64:x\nDB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3399\nDB_DATABASE=shop\nDB_USERNAME=root\nDB_PASSWORD=lerd\n")
	envKeepForeignDB = true
	if err := runEnv(nil, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(envOf(t, dir), "DB_PORT=3399") || !hasDB(calls) {
		t.Errorf("did not follow the port:\n%s\n%+v", envOf(t, dir), calls)
	}
}

func TestRunEnv_ARunTheUserAskedForStillWiresLerd(t *testing.T) {
	dir, calls := foreignDBSite(t, dbnginEnv)
	envKeepForeignDB = false
	if err := runEnv(nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(envOf(t, dir), "DB_PASSWORD=lerd\n") || !hasDB(calls) {
		t.Errorf("not wired:\n%s\n%+v", envOf(t, dir), calls)
	}
}

// A worktree on lerd's database must not be handed the parent's kept server.
func TestKeptDBPinned_KeepsTheConnectionFromWorktrees(t *testing.T) {
	dir, _ := foreignDBSite(t, dbnginEnv)
	kept := foreignDBToKeep(dir, true)
	fw, _ := config.GetFrameworkForDir("laravel", dir)
	carried := carriedToWorktrees(map[string]string{"DB_HOST": "127.0.0.1", "DB_PASSWORD": "", "REDIS_HOST": "127.0.0.1"}, kept.pinned(fw, nil))
	if _, ok := carried["DB_HOST"]; ok || carried["REDIS_HOST"] == "" {
		t.Errorf("carried = %v", carried)
	}
}

func TestEnvCommandFor_KeepsAForeignDatabase(t *testing.T) {
	if args := envCommandFor("/p/shop").Args; !slices.Contains(args, "--"+keepForeignDBFlag) {
		t.Errorf("args = %v", args)
	}
	if NewEnvCmd().Flags().Lookup(keepForeignDBFlag) == nil {
		t.Error("lerd env does not accept the flag")
	}
}

// A port move names the sites, and the connections it keeps, before any is touched.
func TestRefreshHostProxySites_NamesTheSitesBeforeRewritingThem(t *testing.T) {
	foreignDBSite(t, dbnginEnv)
	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()
	prev := portMoveEnvRefresh
	t.Cleanup(func() { portMoveEnvRefresh = prev })
	var before string
	portMoveEnvRefresh = func(string) error { before = buf.String(); return nil }

	if err := config.SetSiteWiredServices("shop", []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	refreshHostProxySitesForService("mysql")
	if !strings.Contains(before, "rewriting .env for shop") || !strings.Contains(before, "shop (127.0.0.1:3306)") {
		t.Errorf("announced %q", before)
	}
}

// Each automatic seam in production re-execs with the flag, proven on a stand-in
// for the spawn all of them share.
func TestAutomaticSeams_PassTheFlag(t *testing.T) {
	prev := spawnLerdEnv
	t.Cleanup(func() { spawnLerdEnv = prev })
	var got [][]string
	spawnLerdEnv = func(_ string, _ io.Writer, flags []string) error {
		got = append(got, flags)
		return errors.New("stop")
	}
	_ = portMoveEnvRefresh("/p")
	_ = hostProxyEnvRefresh("/p")
	_ = domainSyncEnvFn("/p", io.Discard)
	_ = runLerdEnv("/p") // db:move must not keep
	if len(got) != 4 {
		t.Fatalf("spawned %d", len(got))
	}
	for i, flags := range got {
		if keeps := slices.Contains(flags, "--"+keepForeignDBFlag); keeps != (i < 3) {
			t.Errorf("seam %d flags = %v", i, flags)
		}
	}
}
