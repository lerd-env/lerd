package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// alignWorktreeEnvs mirrors the parent's DB connection coordinates
// into each worktree .env (the worktree arm of `lerd env`), while leaving the
// worktree-specific DB_DATABASE alone.
func TestAlignWorktreeEnvs_realignsHostKeepsDatabase(t *testing.T) {
	main := t.TempDir()
	checkout := t.TempDir()

	// Main repo .git dir so IsMainRepo is true, plus the worktree metadata that
	// DetectWorktrees reads (HEAD for the branch, gitdir for the checkout path).
	wtMeta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat/add-social-logins\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Parent .env already aligned to the current service; worktree .env is stale.
	mainEnv := "DB_CONNECTION=pgsql\nDB_HOST=lerd-postgres-18\nDB_PORT=5432\nDB_DATABASE=acme\n"
	if err := os.WriteFile(filepath.Join(main, ".env"), []byte(mainEnv), 0644); err != nil {
		t.Fatal(err)
	}
	wtEnv := "DB_CONNECTION=pgsql\nDB_HOST=lerd-postgres\nDB_PORT=5432\nDB_DATABASE=acme_feat_add_social_logins\n"
	if err := os.WriteFile(filepath.Join(checkout, ".env"), []byte(wtEnv), 0644); err != nil {
		t.Fatal(err)
	}

	site := &config.Site{Name: "acme", Path: main, Domains: []string{"acme.test"}}
	alignWorktreeEnvs(site, nil, filepath.Join(main, ".env"), ".env", "", nil, nil)

	got, err := os.ReadFile(filepath.Join(checkout, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "DB_HOST=lerd-postgres-18") {
		t.Errorf("worktree DB_HOST not realigned to parent:\n%s", s)
	}
	if strings.Contains(s, "DB_HOST=lerd-postgres\n") {
		t.Errorf("stale worktree DB_HOST still present:\n%s", s)
	}
	if !strings.Contains(s, "DB_DATABASE=acme_feat_add_social_logins") {
		t.Errorf("worktree-specific DB_DATABASE must be left untouched:\n%s", s)
	}
}

// A worktree that has no .env yet is skipped without error.
func TestAlignWorktreeEnvs_skipsWorktreeWithoutEnv(t *testing.T) {
	main := t.TempDir()
	checkout := t.TempDir()

	wtMeta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)
	os.WriteFile(filepath.Join(main, ".env"), []byte("DB_HOST=lerd-postgres-18\n"), 0644)

	site := &config.Site{Name: "acme", Path: main, Domains: []string{"acme.test"}}
	// No worktree .env: must be a no-op, no panic, no file created.
	alignWorktreeEnvs(site, nil, filepath.Join(main, ".env"), ".env", "", nil, nil)

	if _, err := os.Stat(filepath.Join(checkout, ".env")); !os.IsNotExist(err) {
		t.Error("worktree .env should not have been created")
	}
}

// A runtime switch moves the parent's service hosts to loopback. The worktree
// still holds the old hosts and follows, while its own database and URL, which
// never equalled the parent's, stay as they are.
func TestAlignWorktreeEnvs_followsWhatTheParentMoved(t *testing.T) {
	main := t.TempDir()
	checkout := t.TempDir()

	wtMeta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)

	before := map[string]string{
		"APP_URL": "http://acme.test", "DB_HOST": "lerd-mysql", "DB_DATABASE": "acme",
		"REDIS_HOST": "lerd-redis", "REDIS_PORT": "6379",
	}
	mainEnv := "APP_URL=https://acme.test\nDB_HOST=127.0.0.1\nDB_DATABASE=acme\nREDIS_HOST=127.0.0.1\nREDIS_PORT=6379\n"
	os.WriteFile(filepath.Join(main, ".env"), []byte(mainEnv), 0644)
	wtEnv := "APP_URL=http://feat.acme.test\nDB_HOST=lerd-mysql\nDB_DATABASE=acme_feat\nREDIS_HOST=lerd-redis\nREDIS_PORT=6379\n"
	os.WriteFile(filepath.Join(checkout, ".env"), []byte(wtEnv), 0644)

	site := &config.Site{Name: "acme", Path: main, Domains: []string{"acme.test"}}
	alignWorktreeEnvs(site, nil, filepath.Join(main, ".env"), ".env", "", before, nil)

	got, err := os.ReadFile(filepath.Join(checkout, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	want := "APP_URL=http://feat.acme.test\nDB_HOST=127.0.0.1\nDB_DATABASE=acme_feat\nREDIS_HOST=127.0.0.1\nREDIS_PORT=6379\n"
	if string(got) != want {
		t.Errorf("worktree .env =\n%s\nwant\n%s", got, want)
	}
}

// A parent's .env.lerd_override is personal to that checkout, so a value it
// changed is not handed on to the worktrees.
func TestCarriedToWorktrees_leavesOverridesBehind(t *testing.T) {
	before := map[string]string{"REDIS_HOST": "lerd-redis", "SESSION_DOMAIN": ".acme.test"}
	got := carriedToWorktrees(before, map[string]string{"SESSION_DOMAIN": ".mine.test"})
	if _, ok := got["SESSION_DOMAIN"]; ok || got["REDIS_HOST"] != "lerd-redis" {
		t.Errorf("carriedToWorktrees = %v, want REDIS_HOST only", got)
	}
	if before["SESSION_DOMAIN"] == "" {
		t.Error("the parent's own map must not be modified")
	}
}

// lerd env run inside a worktree used to name the database after the checkout
// folder, since a worktree is not a registered site, and create it empty.
func TestWorktreeEnvTarget(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	site := &config.Site{Name: "acme", Path: filepath.Join(t.TempDir(), "acme"), Domains: []string{"acme.test"}}

	db, domain := worktreeEnvTarget(site, "feat-x")
	if db != "acme" || domain != "feat-x.acme.test" {
		t.Errorf("shared worktree = (%q, %q), want (acme, feat-x.acme.test)", db, domain)
	}

	if err := config.AddWorktreeDB(config.WorktreeDBEntry{Site: "acme", Branch: "feat-x", Service: "mysql", DBName: "acme_feat_x"}); err != nil {
		t.Fatal(err)
	}
	if db, _ := worktreeEnvTarget(site, "feat-x"); db != "acme_feat_x" {
		t.Errorf("isolated worktree database = %q, want acme_feat_x", db)
	}
}

// A project keeping its configuration in a PHP file has worktrees like any
// other, and they drifted the same way. The align arm read and wrote dotenv
// only, so it returned before touching them and their service hosts stayed on
// a container name for good.
func TestAlignWorktreeEnvs_alignsAPhpArrayProject(t *testing.T) {
	main := t.TempDir()
	checkout := t.TempDir()

	wtMeta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)

	const envRel = "app/etc/env.php"
	if err := os.MkdirAll(filepath.Join(main, "app", "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(checkout, "app", "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	const phpTmpl = "<?php\nreturn [\n    'db' => [\n        'connection' => [\n            'default' => [\n                'host' => '%s',\n            ],\n        ],\n    ],\n];\n"
	os.WriteFile(filepath.Join(main, envRel), []byte(fmt.Sprintf(phpTmpl, "127.0.0.1")), 0644)
	os.WriteFile(filepath.Join(checkout, envRel), []byte(fmt.Sprintf(phpTmpl, "lerd-mysql")), 0644)

	before := map[string]string{"db.connection.default.host": "lerd-mysql"}
	site := &config.Site{Name: "acme", Path: main, Domains: []string{"acme.test"}}
	alignWorktreeEnvs(site, nil, filepath.Join(main, envRel), envRel, "php-array", before, nil)

	got, err := os.ReadFile(filepath.Join(checkout, envRel))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "lerd-mysql") {
		t.Errorf("worktree still names the container:\n%s", got)
	}
	if !strings.Contains(string(got), "127.0.0.1") {
		t.Errorf("worktree did not follow the parent:\n%s", got)
	}
}

// A value the parent's .env.lerd_override pins lands in the parent's .env, but
// it is personal to that checkout: a worktree still naming the container, or
// holding its own DB host, must not be pointed at it.
func TestAlignWorktreeEnvs_leavesTheParentsOverridesBehind(t *testing.T) {
	main := t.TempDir()
	checkout := t.TempDir()

	wtMeta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(wtMeta, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wtMeta, "HEAD"), []byte("ref: refs/heads/feat\n"), 0644)
	os.WriteFile(filepath.Join(wtMeta, "gitdir"), []byte(filepath.Join(checkout, ".git")+"\n"), 0644)

	overrides := map[string]string{"REDIS_HOST": "redis.example.com", "DB_HOST": "db.example.com"}
	before := map[string]string{"DB_HOST": "db.example.com", "REDIS_HOST": "redis.example.com", "REDIS_PORT": "6379"}
	os.WriteFile(filepath.Join(main, ".env"), []byte("DB_HOST=db.example.com\nREDIS_HOST=redis.example.com\nREDIS_PORT=6379\n"), 0644)
	wtEnv := "DB_HOST=lerd-mysql\nREDIS_HOST=lerd-redis\nREDIS_PORT=6379\n"
	os.WriteFile(filepath.Join(checkout, ".env"), []byte(wtEnv), 0644)

	site := &config.Site{Name: "acme", Path: main, Domains: []string{"acme.test"}}
	alignWorktreeEnvs(site, fwWithServices(), filepath.Join(main, ".env"), ".env", "", before, overrides)

	got, err := os.ReadFile(filepath.Join(checkout, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wtEnv {
		t.Errorf("worktree .env =\n%s\nwant it untouched:\n%s", got, wtEnv)
	}
}
