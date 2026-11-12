package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// makeFakeWorktree creates a minimal main-repo + worktree fixture on disk and
// returns (sitePath, worktreeCheckoutPath).
func makeFakeWorktree(t *testing.T, branch string) (string, string) {
	t.Helper()
	siteDir := t.TempDir()
	gitDir := filepath.Join(siteDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	wtMeta := filepath.Join(gitDir, "worktrees", branch)
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), branch+"-checkout")
	if err := os.Mkdir(checkout, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/"+branch+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return siteDir, checkout
}

// FindParentSiteForWorktree must locate the registered parent site whose
// .git/worktrees/<branch> contains the given checkout dir.
func TestFindParentSiteForWorktree_resolvesParent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath, checkout := makeFakeWorktree(t, "feat-a")
	if err := config.AddSite(config.Site{
		Name:    "acme",
		Path:    sitePath,
		Domains: []string{"acme.test"},
	}); err != nil {
		t.Fatal(err)
	}

	site, branch, ok := FindParentSiteForWorktree(checkout)
	if !ok {
		t.Fatal("FindParentSiteForWorktree returned ok=false; want match")
	}
	if site.Name != "acme" {
		t.Errorf("site = %q, want %q", site.Name, "acme")
	}
	if branch != "feat-a" {
		t.Errorf("branch = %q, want %q", branch, "feat-a")
	}
}

// A checkout reached through a symlinked parent (/home on ostree hosts) is the
// same worktree git recorded under its real path.
func TestFindParentSiteForWorktree_symlinkedSpelling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath, checkout := makeFakeWorktree(t, "feat-a")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "home")
	if err := os.Symlink(filepath.Dir(checkout), link); err != nil {
		t.Fatal(err)
	}

	site, _, ok := FindParentSiteForWorktree(filepath.Join(link, filepath.Base(checkout)))
	if !ok || site.Name != "acme" {
		t.Fatalf("FindParentSiteForWorktree via the symlink = %v, %v; want acme", site, ok)
	}
}

// lerd worktree wait holds a worktree reached through a symlinked parent until
// its vhost exists, the same as one spelled by its real path.
func TestWorktreeServed_symlinkedSpelling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	sitePath, checkout := makeFakeWorktree(t, "feat-a")
	if err := config.AddSite(config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "home")
	if err := os.Symlink(filepath.Dir(checkout), link); err != nil {
		t.Fatal(err)
	}

	if worktreeServed(filepath.Join(link, filepath.Base(checkout))) {
		t.Error("worktreeServed = true with no vhost written; want the wait held")
	}
}

// A directory that is not a worktree of any registered site returns ok=false.
func TestFindParentSiteForWorktree_unregisteredDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if _, _, ok := FindParentSiteForWorktree(t.TempDir()); ok {
		t.Errorf("FindParentSiteForWorktree returned ok=true for unrelated tempdir")
	}
}

// WorktreeDBName matches the projectDBName underscore convention.
func TestWorktreeDBName_underscoreSlug(t *testing.T) {
	cases := map[[2]string]string{
		{"acme", "feat-a"}:       "acme_feat_a",
		{"my_app", "feature/x"}:  "my_app_feature_x", // SiteSlug maps the slash to _ along with every other non-[a-z0-9_] char
		{"acme_app", "feat-x-y"}: "acme_app_feat_x_y",
		{"acme", "RELEASE-1"}:    "acme_release_1",
	}
	for in, want := range cases {
		if got := WorktreeDBName(in[0], in[1]); got != want {
			t.Errorf("WorktreeDBName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

// A worktree's running workers hold the database connection they started with,
// so moving the worktree to another database restarts them onto the new one.
func TestSetWorktreeDBIsolated_restartsTheWorktreesWorkers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	sitePath, checkout := makeFakeWorktree(t, "feature")
	for _, dir := range []string{sitePath, checkout} {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\nDB_DATABASE=acme\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var restarted []string
	orig := restartWorktreeWorkersFn
	restartWorktreeWorkersFn = func(_ *config.Site, wtPath string) error { restarted = append(restarted, wtPath); return nil }
	t.Cleanup(func() { restartWorktreeWorkersFn = orig })

	site := &config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}
	if err := SetWorktreeDBIsolated(site, "feature", false, ""); err != nil {
		t.Fatalf("SetWorktreeDBIsolated: %v", err)
	}
	if len(restarted) != 1 || restarted[0] != checkout {
		t.Fatalf("restarted %v, want the worktree at %s", restarted, checkout)
	}
}

// A service that declares no way to drop a database must not report the
// worktree's database as dropped while it is still there.
func TestDropWorktreeDB_refusesAServiceWithNoDropAction(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	dir := filepath.Join(cfg, "lerd", "services")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "myengine.yaml"), []byte("name: myengine\nimage: example/engine:1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err := dropWorktreeDB(config.WorktreeDBEntry{Site: "acme", Branch: "feature", Service: "myengine", DBName: "acme_feature"})
	if err == nil {
		t.Fatal("dropWorktreeDB returned nil for a service that cannot drop the database")
	}
}

// Isolating on a service that cannot make and drop databases would point the
// worktree at a database lerd never created and could never remove.
func TestSetWorktreeDBIsolated_refusesAServiceWithoutDatabaseActions(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join(cfg, "lerd", "services")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "myengine.yaml"), []byte("name: myengine\nimage: example/engine:1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sitePath, checkout := makeFakeWorktree(t, "feature")
	for _, d := range []string{sitePath, checkout} {
		if err := os.WriteFile(filepath.Join(d, ".env"), []byte("DB_HOST=lerd-myengine\nDB_DATABASE=acme\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	created := false
	orig := createDatabaseFn
	createDatabaseFn = func(string, string) (bool, error) { created = true; return false, nil }
	t.Cleanup(func() { createDatabaseFn = orig })

	site := &config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}
	err := SetWorktreeDBIsolated(site, "feature", true, "")
	if err == nil || created {
		t.Fatalf("want a refusal before any create, got err=%v created=%v", err, created)
	}
}

// writeFamilyOnlyService installs an inline service naming only its family and
// a fetch hook that serves that family's preset, recording what it fetched.
func writeFamilyOnlyService(t *testing.T) *[]string {
	t.Helper()
	cfg, data := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	dir := filepath.Join(cfg, "lerd", "services")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acmedb-app.yaml"), []byte("name: acmedb-app\nimage: example/acmedb:1\nfamily: acmedb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	preset := "name: acmedb\nimage: example/acmedb:1\nfamily: acmedb\nintrospect:\n  entities:\n    - kind: databases\n      list: list-dbs\n      actions:\n        create: make {{name}}\n        drop: drop {{name}}\n"
	presets := filepath.Join(data, "lerd", "service-presets")
	var fetched []string
	config.RegisterPresetFetchHook(func(name string) error {
		fetched = append(fetched, name)
		if err := os.MkdirAll(presets, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(presets, name+".yaml"), []byte(preset), 0644)
	})
	t.Cleanup(func() { config.RegisterPresetFetchHook(nil) })
	return &fetched
}

func TestSetWorktreeDBIsolated_fetchesTheFamilyPreset(t *testing.T) {
	fetched := writeFamilyOnlyService(t)
	sitePath, checkout := makeFakeWorktree(t, "feature")
	for _, d := range []string{sitePath, checkout} {
		if err := os.WriteFile(filepath.Join(d, ".env"), []byte("DB_HOST=lerd-acmedb-app\nDB_DATABASE=acme\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var created string
	origCreate, origRestart := createDatabaseFn, restartWorktreeWorkersFn
	createDatabaseFn = func(_, name string) (bool, error) { created = name; return true, nil }
	restartWorktreeWorkersFn = func(*config.Site, string) error { return nil }
	t.Cleanup(func() { createDatabaseFn, restartWorktreeWorkersFn = origCreate, origRestart })

	site := &config.Site{Name: "acme", Path: sitePath, Domains: []string{"acme.test"}}
	if err := SetWorktreeDBIsolated(site, "feature", true, ""); err != nil {
		t.Fatalf("SetWorktreeDBIsolated: %v", err)
	}
	if len(*fetched) != 1 || created != "acme_feature" {
		t.Fatalf("fetched %v, created %q; want the acmedb preset fetched and acme_feature created", *fetched, created)
	}
}

func TestDropWorktreeDB_fetchesTheFamilyPreset(t *testing.T) {
	fetched := writeFamilyOnlyService(t)
	err := dropWorktreeDB(config.WorktreeDBEntry{Site: "acme", Branch: "feature", Service: "acmedb-app", DBName: "acme_feature"})
	if len(*fetched) != 1 || (*fetched)[0] != "acmedb" {
		t.Fatalf("fetched %v, want [acmedb]", *fetched)
	}
	if err != nil && strings.Contains(err.Error(), "declares no way") {
		t.Fatalf("the fetched preset's drop must be used, got %v", err)
	}
}
