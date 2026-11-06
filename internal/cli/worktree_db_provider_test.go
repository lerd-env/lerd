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

// A database kept from a removed worktree belongs to whoever made it. Another
// plugin in charge now must not branch a new one over its record.
func TestApplyWorktreeDBChoice_ReuseRefusesADatabaseAnotherPluginMade(t *testing.T) {
	site, checkout, calls := providerFixture(t, `echo '{"database":"acme_feature_br"}'`+"\n")
	kept := config.WorktreeDBEntry{Site: "acme", Branch: "feature", Service: "mysql", DBName: "kept_other", Host: "lerd-other", Provider: "other"}
	if err := config.AddWorktreeDB(kept); err != nil {
		t.Fatal(err)
	}
	err := ApplyWorktreeDBChoice(site, "feature", "reuse", nil)
	if err == nil || !strings.Contains(err.Error(), "other") {
		t.Fatalf("err = %v, want the plugin that made it named", err)
	}
	if log := readFile(t, calls); log != "" {
		t.Errorf("the plugin in charge must not be asked for a new database, got %q", log)
	}
	if got, ok, _ := config.FindWorktreeDB("acme", "feature"); !ok || got != kept {
		t.Errorf("the kept entry must stay as it was, got %+v %v", got, ok)
	}
	if env := readFile(t, filepath.Join(checkout, ".env")); env != "DB_HOST=lerd-mysql\nDB_DATABASE=acme\n" {
		t.Errorf("env changed to %q", env)
	}
}

// The same holds when the plugin that made the kept database is gone: lerd
// must not make an empty one of its own and drop the record of the real one.
func TestApplyWorktreeDBChoice_ReuseRefusesADatabaseOfAGonePlugin(t *testing.T) {
	site, _, _ := providerFixture(t, "")
	if err := plugins.Remove("brancher"); err != nil {
		t.Fatal(err)
	}
	kept := config.WorktreeDBEntry{Site: "acme", Branch: "feature", Service: "mysql", DBName: "kept_br", Host: "lerd-brancher", Provider: "brancher"}
	if err := config.AddWorktreeDB(kept); err != nil {
		t.Fatal(err)
	}
	if err := ApplyWorktreeDBChoice(site, "feature", "reuse", nil); err == nil || !strings.Contains(err.Error(), "brancher") {
		t.Fatalf("err = %v, want the gone plugin named", err)
	}
	if got, ok, _ := config.FindWorktreeDB("acme", "feature"); !ok || got != kept {
		t.Errorf("the kept entry must stay as it was, got %+v %v", got, ok)
	}
}

// A database lerd kept before a plugin took charge is still lerd's, so reuse
// reconnects to it rather than asking the plugin for a new one.
func TestApplyWorktreeDBChoice_ReuseReconnectsToLerdsOwnDatabase(t *testing.T) {
	site, checkout, calls := providerFixture(t, `echo '{"database":"acme_feature_br"}'`+"\n")
	kept := config.WorktreeDBEntry{Site: "acme", Branch: "feature", Service: "mysql", DBName: "acme_feature"}
	if err := config.AddWorktreeDB(kept); err != nil {
		t.Fatal(err)
	}
	if err := ApplyWorktreeDBChoice(site, "feature", "reuse", nil); err != nil {
		t.Fatal(err)
	}
	if log := readFile(t, calls); log != "" {
		t.Errorf("the plugin must not be asked for a new database, got %q", log)
	}
	if got, _, _ := config.FindWorktreeDB("acme", "feature"); got != kept {
		t.Errorf("entry = %+v, want lerd's kept one", got)
	}
	if env := readFile(t, filepath.Join(checkout, ".env")); !strings.Contains(env, "DB_DATABASE=acme_feature\n") {
		t.Errorf("env = %q", env)
	}
}

// A drop the plugin fails leaves the worktree on its own database and the
// record in place, so nothing is left behind unaccounted for.
func TestSetWorktreeDBIsolated_FailedDropChangesNothing(t *testing.T) {
	script := "case \"$1\" in drop) echo 'busy' >&2; exit 1;; esac\necho '{\"host\":\"lerd-brancher\",\"database\":\"acme_feature_br\"}'\n"
	site, checkout, _ := providerFixture(t, script)
	if err := SetWorktreeDBIsolated(site, "feature", true, "main"); err != nil {
		t.Fatalf("isolate: %v", err)
	}
	before := readFile(t, filepath.Join(checkout, ".env"))

	err := SetWorktreeDBIsolated(site, "feature", false, "")
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v, want the plugin's error", err)
	}
	if _, ok, _ := config.FindWorktreeDB("acme", "feature"); !ok {
		t.Error("the record must be kept when the drop failed")
	}
	if env := readFile(t, filepath.Join(checkout, ".env")); env != before {
		t.Errorf("env changed to %q", env)
	}
}

// Orphan cleanup forgets a database only once it is actually dropped.
func TestDropOrphanedWorktreeDBs_KeepsTheRecordWhenTheDropFails(t *testing.T) {
	site, _, _ := providerFixture(t, "echo 'busy' >&2\nexit 1\n")
	entry := config.WorktreeDBEntry{Site: "acme", Branch: "gone", Service: "mysql", DBName: "acme_gone_br", Provider: "brancher"}
	if err := config.AddWorktreeDB(entry); err != nil {
		t.Fatal(err)
	}
	DropOrphanedWorktreeDBs(site)
	if _, ok, _ := config.FindWorktreeDB("acme", "gone"); !ok {
		t.Error("the record of a database still out there must be kept")
	}
}

// DB_HOST=127.0.0.1 with mysql in .lerd.yaml reads as lerd's container, so a
// server on the machine used to get its worktree cloned into the wrong one.
func TestSetWorktreeDBIsolated_LoopbackOnAnotherPortIsNotLerds(t *testing.T) {
	site, checkout, _ := providerFixture(t, "")
	removeProvider(t)
	for _, dir := range []string{site.Path, checkout} {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=acme\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(site.Path, ".lerd.yaml"), []byte("services:\n  - mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
		if c.Services == nil {
			c.Services = map[string]config.ServiceConfig{}
		}
		sc := c.Services["mysql"]
		sc.Port = 3308
		c.Services["mysql"] = sc
	}); err != nil {
		t.Fatal(err)
	}
	created := ""
	orig := createDatabaseFn
	createDatabaseFn = func(service, name string) (bool, error) {
		created = service + "/" + name
		return true, nil
	}
	t.Cleanup(func() { createDatabaseFn = orig })

	err := SetWorktreeDBIsolated(site, "feature", true, "empty")
	if err == nil || !strings.Contains(err.Error(), "not on a lerd service") {
		t.Fatalf("got %v", err)
	}
	if created != "" {
		t.Errorf("lerd created %s", created)
	}
}

func removeProvider(t *testing.T) {
	t.Helper()
	if err := plugins.Remove("brancher"); err != nil {
		t.Fatal(err)
	}
}

// urlFrameworkFixture is a worktree of a site whose framework keeps its whole
// connection in DATABASE_URL.
func urlFrameworkFixture(t *testing.T) (*config.Site, string) {
	t.Helper()
	site, checkout, _ := providerFixture(t, "")
	removeProvider(t)
	writeURLFramework(t, site.Path, checkout)
	return site, checkout
}

func writeURLFramework(t *testing.T, dirs ...string) {
	t.Helper()
	if err := config.SaveStoreFramework(&config.Framework{
		Name:   "acme",
		Detect: []config.FrameworkRule{{File: "acme.lock"}},
		Env: config.FrameworkEnvConf{
			File: ".env.local",
			Services: map[string]config.FrameworkServiceDef{
				"mysql": {
					Detect: []config.FrameworkServiceDetect{{Key: "DATABASE_URL", ValuePrefix: "mysql"}},
					Vars:   []string{"DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/{{site}}?serverVersion=8.4"},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		for name, body := range map[string]string{
			"acme.lock":  "",
			".env.local": "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/acme?serverVersion=8.4\n",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSetWorktreeDBIsolated_RenamesTheDatabaseInAConnectionURL(t *testing.T) {
	site, checkout := urlFrameworkFixture(t)
	orig := createDatabaseFn
	created := ""
	createDatabaseFn = func(service, name string) (bool, error) {
		created = service + "/" + name
		return true, nil
	}
	t.Cleanup(func() { createDatabaseFn = orig })

	if err := SetWorktreeDBIsolated(site, "feature", true, "empty"); err != nil {
		t.Fatal(err)
	}
	if created != "mysql/acme_feature" {
		t.Errorf("created %q", created)
	}
	if got := readFile(t, filepath.Join(checkout, ".env.local")); !strings.Contains(got, "@lerd-mysql:3306/acme_feature?serverVersion=8.4") {
		t.Errorf("worktree env = %s", got)
	}
}

// A worktree's own connection options are not the parent's to replace.
func TestSetWorktreeDBIsolated_KeepsTheWorktreesOwnURLOptions(t *testing.T) {
	site, checkout := urlFrameworkFixture(t)
	orig := createDatabaseFn
	createDatabaseFn = func(string, string) (bool, error) { return true, nil }
	t.Cleanup(func() { createDatabaseFn = orig })
	env := filepath.Join(checkout, ".env.local")
	if err := os.WriteFile(env, []byte("DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/acme?serverVersion=9.7&timeout=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetWorktreeDBIsolated(site, "feature", true, "empty"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, env); !strings.Contains(got, "/acme_feature?serverVersion=9.7&timeout=5") {
		t.Errorf("isolating: %s", got)
	}
	if _, _, err := config.RemoveWorktreeDB(site.Name, "feature"); err != nil {
		t.Fatal(err)
	}
	if err := SetWorktreeDBIsolated(site, "feature", false, ""); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, env); !strings.Contains(got, "/acme?serverVersion=9.7&timeout=5") {
		t.Errorf("sharing: %s", got)
	}
}

// A plugin serving from another host cannot be written into a URL lerd only
// renames the database in, and pointing the branch at the wrong server is worse.
func TestConnectWorktreeDB_RefusesAHostItCannotWriteIntoAURL(t *testing.T) {
	site, checkout := urlFrameworkFixture(t)
	err := connectWorktreeDB(site, checkout, config.WorktreeDBEntry{DBName: "acme_feature", Host: "branch.example"})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("got %v", err)
	}
	if got := readFile(t, filepath.Join(checkout, ".env.local")); strings.Contains(got, "acme_feature") {
		t.Errorf("worktree env changed: %s", got)
	}
	if config.WorktreeDBIsolated(checkout) {
		t.Error("a refused worktree still shares the parent's database and must say so")
	}
}

func TestIsolateWithPlugin_UndoesADatabaseItCannotConnect(t *testing.T) {
	site, checkout, calls := providerFixture(t, `case "$1" in create) echo '{"database":"acme_feature","host":"branch.example"}';; esac`+"\n")
	writeURLFramework(t, site.Path, checkout)
	if err := SetWorktreeDBIsolated(site, "feature", true, "empty"); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("got %v", err)
	}
	if _, ok, _ := config.FindWorktreeDB(site.Name, "feature"); ok {
		t.Error("a refused database must not stay recorded")
	}
	if config.WorktreeDBIsolated(checkout) {
		t.Error("a refused worktree still shares the parent's database and must say so")
	}
	if got := readFile(t, calls); !strings.Contains(got, "drop ") {
		t.Errorf("the plugin was left holding the database: %s", got)
	}
}

// The copy goes on the parent's server, so a worktree pointed elsewhere would
// report isolated while connected to a server that does not have it.
func TestSetWorktreeDBIsolated_RefusesAWorktreeOnAnotherServer(t *testing.T) {
	site, checkout := urlFrameworkFixture(t)
	env := filepath.Join(checkout, ".env.local")
	const elsewhere = "DATABASE_URL=mysql://me:pw@db.elsewhere:3306/acme\n"
	if err := os.WriteFile(env, []byte(elsewhere), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := createDatabaseFn
	created := false
	createDatabaseFn = func(string, string) (bool, error) {
		created = true
		return true, nil
	}
	t.Cleanup(func() { createDatabaseFn = orig })

	err := SetWorktreeDBIsolated(site, "feature", true, "empty")
	if err == nil || !strings.Contains(err.Error(), "another server") {
		t.Fatalf("got %v", err)
	}
	if created || config.WorktreeDBIsolated(checkout) {
		t.Errorf("created=%v isolated=%v, want nothing changed", created, config.WorktreeDBIsolated(checkout))
	}
	if got := readFile(t, env); got != elsewhere {
		t.Errorf("env changed: %s", got)
	}
}

// A database the plugin could not drop has to stay findable, or nothing ever
// cleans it up. db:share is what clears it later.
func TestIsolateWithPlugin_KeepsARecordWhenItCannotDrop(t *testing.T) {
	site, checkout, _ := providerFixture(t, `case "$1" in create) echo '{"database":"acme_feature","host":"branch.example"}';; drop) echo cannot >&2; exit 1;; esac`+"\n")
	writeURLFramework(t, site.Path, checkout)

	err := SetWorktreeDBIsolated(site, "feature", true, "empty")
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") || !strings.Contains(err.Error(), "acme_feature") {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "cannot") {
		t.Errorf("the failed cleanup must be reported: %v", err)
	}
	if _, ok, _ := config.FindWorktreeDB(site.Name, "feature"); !ok {
		t.Error("the leftover database lost its record")
	}
	if config.WorktreeDBIsolated(checkout) {
		t.Error("the worktree still shares the parent's database and must say so")
	}
}
