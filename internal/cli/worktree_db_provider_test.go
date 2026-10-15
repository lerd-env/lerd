package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// providerFixture is a worktree sharing acme's MySQL database and a plugin in
// charge of worktree databases running script. It returns the site, the
// worktree checkout and the file the plugin logs its calls to.
func providerFixture(t *testing.T, script string) (*config.Site, string, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath, checkout := makeFakeWorktree(t, "feature")
	for _, dir := range []string{sitePath, checkout} {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\nDB_DATABASE=acme\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	orig := restartWorktreeWorkersFn
	restartWorktreeWorkersFn = func(*config.Site, string) error { return nil }
	t.Cleanup(func() { restartWorktreeWorkersFn = orig })

	dir := filepath.Join(plugins.Dir(), "brancher")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(t.TempDir(), "calls")
	manifest := "api_version: 1\nname: brancher\nprovides:\n  worktree_database: bin/db\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nread -r req\necho \"$1 $req\" >> " + calls + "\n" + script
	if err := os.WriteFile(filepath.Join(dir, "bin", "db"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return &config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}, checkout, calls
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestSetWorktreeDBIsolated_HandsTheDatabaseToThePluginInCharge(t *testing.T) {
	site, checkout, calls := providerFixture(t, `echo '{"host":"lerd-brancher","database":"acme_feature_br"}'`+"\n")

	if err := SetWorktreeDBIsolated(site, "feature", true, "main"); err != nil {
		t.Fatalf("isolate: %v", err)
	}
	log := readFile(t, calls)
	if !strings.HasPrefix(log, "create ") || !strings.Contains(log, `"source":"main"`) || !strings.Contains(log, `"database":"acme_feature"`) {
		t.Errorf("the plugin got %q", log)
	}
	if env := readFile(t, filepath.Join(checkout, ".env")); !strings.Contains(env, "DB_HOST=lerd-brancher") || !strings.Contains(env, "DB_DATABASE=acme_feature_br") {
		t.Errorf("worktree env = %q", env)
	}
	entry, ok, _ := config.FindWorktreeDB("acme", "feature")
	if !ok || entry.Provider != "brancher" || entry.DBName != "acme_feature_br" {
		t.Errorf("registry = %+v %v", entry, ok)
	}

	if err := SetWorktreeDBIsolated(site, "feature", false, ""); err != nil {
		t.Fatalf("share again: %v", err)
	}
	if log := readFile(t, calls); !strings.Contains(log, "drop ") || !strings.Contains(log, `"database":"acme_feature_br"`) {
		t.Errorf("the plugin must drop what it made, got %q", log)
	}
	if env := readFile(t, filepath.Join(checkout, ".env")); !strings.Contains(env, "DB_HOST=lerd-mysql") || !strings.Contains(env, "DB_DATABASE=acme\n") {
		t.Errorf("sharing again must restore the parent's host and database, env = %q", env)
	}
}

// A failure leaves the worktree on the shared database, never on a full copy
// lerd makes behind the plugin's back.
func TestSetWorktreeDBIsolated_PluginFailureKeepsTheWorktreeShared(t *testing.T) {
	cases := map[string]string{
		"no space left": "echo 'no space left' >&2\nexit 1\n",
		`"port"`:        `echo '{"database":"x","port":"3307"}'` + "\n",
		"database":      `echo '{"host":"lerd-brancher"}'` + "\n",
	}
	for want, script := range cases {
		t.Run(want, func(t *testing.T) {
			site, checkout, _ := providerFixture(t, script)
			err := SetWorktreeDBIsolated(site, "feature", true, "main")
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "brancher") {
				t.Fatalf("err = %v, want the plugin and %s named", err, want)
			}
			if env := readFile(t, filepath.Join(checkout, ".env")); env != "DB_HOST=lerd-mysql\nDB_DATABASE=acme\n" {
				t.Errorf("env changed to %q", env)
			}
			if _, ok, _ := config.FindWorktreeDB("acme", "feature"); ok {
				t.Error("nothing may be recorded for a database that was never made")
			}
		})
	}
}

// A database kept from a removed worktree is reconnected to, not replaced by a
// new branch that would leave the kept one behind.
func TestApplyWorktreeDBChoice_ReuseReconnectsToThePluginsDatabase(t *testing.T) {
	site, checkout, calls := providerFixture(t, `echo '{"host":"lerd-brancher","database":"acme_feature_br"}'`+"\n")
	if err := config.AddWorktreeDB(config.WorktreeDBEntry{Site: "acme", Branch: "feature", Service: "mysql", DBName: "kept_br", Host: "lerd-brancher", Provider: "brancher"}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyWorktreeDBChoice(site, "feature", "reuse", nil); err != nil {
		t.Fatal(err)
	}
	if log := readFile(t, calls); log != "" {
		t.Errorf("reuse must not ask the plugin for a new database, it got %q", log)
	}
	if env := readFile(t, filepath.Join(checkout, ".env")); !strings.Contains(env, "DB_HOST=lerd-brancher") || !strings.Contains(env, "DB_DATABASE=kept_br") {
		t.Errorf("env = %q", env)
	}
}
