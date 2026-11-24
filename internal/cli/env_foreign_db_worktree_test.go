package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

const (
	foreignEnv = "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\nDB_DATABASE=my_real_data\nDB_USERNAME=root\nDB_PASSWORD=\n"
	lerdEnv    = "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_PORT=3306\nDB_DATABASE=shop_feat\nDB_USERNAME=root\nDB_PASSWORD=lerd\n"
)

// alignedWorktree runs an automatic env over a site and one worktree, both on
// the same framework, and returns the worktree's values afterwards. file and
// marker are the framework's env file and the file it is detected by.
func alignedWorktree(t *testing.T, framework, marker, file, parentEnv, wtEnv, parentOverride string, hostPort int) map[string]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	stubProvisionSeams(t, nil)
	main, checkout := t.TempDir(), t.TempDir()
	meta := filepath.Join(main, ".git", "worktrees", "feat")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(meta, "HEAD"), "ref: refs/heads/feat\n")
	write(filepath.Join(meta, "gitdir"), filepath.Join(checkout, ".git")+"\n")
	for dir, env := range map[string]string{main: parentEnv, checkout: wtEnv} {
		write(filepath.Join(dir, marker), "")
		write(filepath.Join(dir, file), env)
	}
	if parentOverride != "" {
		write(filepath.Join(main, envOverrideFile), parentOverride)
	}
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: main, Framework: framework, HostPort: hostPort}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	t.Chdir(main)
	prev := envKeepForeignDB
	envKeepForeignDB = true
	t.Cleanup(func() { envKeepForeignDB = prev })
	if err := runEnv(nil, nil); err != nil {
		t.Fatal(err)
	}
	return envfile.Values(filepath.Join(checkout, file), "dotenv")
}

func wantValues(t *testing.T, got map[string]string, want string) {
	t.Helper()
	for _, kv := range strings.Fields(want) {
		k, v, _ := strings.Cut(kv, "=")
		if strings.Trim(got[k], `"`) != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// The parent's server is kept, but a checkout on lerd still follows the runtime.
func TestWorktree_OnLerdFollowsUnderAKeptParent(t *testing.T) {
	got := alignedWorktree(t, "laravel", "artisan", ".env", foreignEnv, lerdEnv, "", 3100)
	wantValues(t, got, "DB_HOST=127.0.0.1 DB_PORT=3306 DB_DATABASE=shop_feat DB_USERNAME=root DB_PASSWORD=lerd")
}

func TestWorktree_OnLerdFollowsBackToContainersUnderAKeptParent(t *testing.T) {
	wt := strings.ReplaceAll(strings.ReplaceAll(lerdEnv, "lerd-mysql", "127.0.0.1"), "3306", "3399")
	got := alignedWorktree(t, "laravel", "artisan", ".env", foreignEnv, wt, "", 0)
	wantValues(t, got, "DB_HOST=lerd-mysql DB_PORT=3306 DB_DATABASE=shop_feat DB_PASSWORD=lerd")
}

// A checkout on a drop-in stays on that service, not the one it stands in for.
func TestWorktree_OnADropInStaysOnItUnderAKeptParent(t *testing.T) {
	wt := strings.ReplaceAll(strings.ReplaceAll(lerdEnv, "lerd-mysql", "lerd-mariadb-11-8"), "3306", "3399")
	got := alignedWorktree(t, "laravel", "artisan", ".env", foreignEnv, wt, "", 0)
	wantValues(t, got, "DB_HOST=lerd-mariadb-11-8 DB_PORT=3306 DB_DATABASE=shop_feat")
}

func TestWorktree_URLFollowsUnderAKeptParent(t *testing.T) {
	got := alignedWorktree(t, "symfony", "symfony.lock", ".env.local",
		"DATABASE_URL=mysql://root:@127.0.0.1:3306/my_real_data\n",
		"DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop_feat\n", "", 3100)
	if u := got["DATABASE_URL"]; !strings.HasPrefix(strings.Trim(u, `"`), "mysql://root:lerd@127.0.0.1:3306/shop_feat") {
		t.Errorf("DATABASE_URL = %q", u)
	}
}

// The mirror of the issue: a checkout on its own server under a parent on lerd.
func TestWorktree_OnAForeignServerIsKeptUnderALerdParent(t *testing.T) {
	got := alignedWorktree(t, "laravel", "artisan", ".env", lerdEnv, foreignEnv, "", 3100)
	wantValues(t, got, "DB_HOST=127.0.0.1 DB_PORT=3306 DB_DATABASE=my_real_data DB_USERNAME=root DB_PASSWORD=")
}

func TestWorktree_OnAForeignServerIsKeptUnderAKeptParent(t *testing.T) {
	got := alignedWorktree(t, "laravel", "artisan", ".env", foreignEnv, strings.ReplaceAll(foreignEnv, "127.0.0.1", "db.example.com"), "", 3100)
	wantValues(t, got, "DB_HOST=db.example.com DB_DATABASE=my_real_data DB_PASSWORD=")
}

// The parent's personal login never reaches a checkout as a file, but it is still
// lerd's server, so the port keeps following.
func TestWorktree_OnLerdWithTheParentsPersonalLoginFollows(t *testing.T) {
	env := "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3399\nDB_DATABASE=shop\nDB_USERNAME=me\nDB_PASSWORD=mine\n"
	got := alignedWorktree(t, "laravel", "artisan", ".env", env, env, "DB_USERNAME=me\nDB_PASSWORD=mine\n", 3100)
	wantValues(t, got, "DB_PORT=3306 DB_USERNAME=me DB_PASSWORD=mine")
}

func TestWorktree_OnSQLiteIsLeftAloneUnderAKeptParent(t *testing.T) {
	got := alignedWorktree(t, "laravel", "artisan", ".env", foreignEnv, "DB_CONNECTION=sqlite\nDB_HOST=lerd-mysql\n", "", 3100)
	wantValues(t, got, "DB_CONNECTION=sqlite")
}
