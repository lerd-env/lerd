package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// registerFrameworkWorktree wires a registered parent site whose framework
// definition declares envFile, plus one worktree checkout, and returns the
// checkout path. envFile "" registers the site without a framework.
func registerFrameworkWorktree(t *testing.T, envFile, format string) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	site := filepath.Join(t.TempDir(), "app")
	wt := filepath.Join(site, "wt", "feature")
	makeWorktree(t, site, wt, "feature")

	framework := ""
	if envFile != "" {
		framework = "envshape"
		store := config.StoreFrameworksDir()
		if err := os.MkdirAll(store, 0755); err != nil {
			t.Fatal(err)
		}
		def := "name: envshape\nlabel: Env Shape\npublic_dir: public\n" +
			"env:\n  file: \"" + envFile + "\"\n  format: " + format + "\n"
		if err := os.WriteFile(filepath.Join(store, "envshape.yaml"), []byte(def), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(site, ".lerd.yaml"), []byte("framework: envshape\n"), 0644); err != nil {
			t.Fatal(err)
		}
		// The parent holds the file the pipeline seeds worktrees from, which is
		// also what Env.Resolve picks between primary and fallback.
		writeFileTree(t, site, envFile)
	}
	if err := config.AddSite(config.Site{Name: "app", Path: site, PHPVersion: "8.4", Framework: framework}); err != nil {
		t.Fatal(err)
	}
	return wt
}

func writeFileTree(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("SEEDED=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// envShapes are the env-file layouts frameworks actually declare: a dotenv at
// the root, a local override, a nested dotenv, and the two php-config shapes.
var envShapes = []struct{ file, format string }{
	{".env", "dotenv"},
	{".env.local", "dotenv"},
	{"config/.env", "dotenv"},
	{"wp-config.php", "php-const"},
	{"app/etc/env.php", "php-array"},
}

func TestWorktreeEnvFile_resolvesThroughTheFrameworkDefinition(t *testing.T) {
	for _, shape := range envShapes {
		wt := registerFrameworkWorktree(t, shape.file, shape.format)
		if got := worktreeEnvFile(wt); got != shape.file {
			t.Errorf("worktreeEnvFile for %s = %q, want %q", shape.file, got, shape.file)
		}
	}
}

func TestWorktreeEnvFile_fallsBackToDotEnv(t *testing.T) {
	// No framework on the parent.
	if got := worktreeEnvFile(registerFrameworkWorktree(t, "", "")); got != ".env" {
		t.Errorf("without a framework: %q, want .env", got)
	}
	// A definition pointing outside the worktree is refused, as the seeding
	// side refuses it.
	if got := worktreeEnvFile(registerFrameworkWorktree(t, "../outside/.env", "dotenv")); got != ".env" {
		t.Errorf("with a non-local env file: %q, want .env", got)
	}
	// A path lerd does not manage as a worktree has no parent to ask.
	if got := worktreeEnvFile(t.TempDir()); got != ".env" {
		t.Errorf("for an unmanaged path: %q, want .env", got)
	}
}

// The regression: the wait hardcoded <worktree>/.env, so a framework seeded
// anywhere else could never satisfy it. A tree with no composer.json and no
// package.json has nothing else to gate on, so the wait burned its whole
// timeout and reported a fully provisioned worktree as unprovisioned.
func TestWaitForWorktreeReady_settlesOnTheFrameworksEnvFile(t *testing.T) {
	for _, shape := range envShapes {
		wt := registerFrameworkWorktree(t, shape.file, shape.format)
		writeFileTree(t, wt, shape.file)

		start := time.Now()
		if err := WaitForWorktreeReady(wt, 4*time.Second); err != nil {
			t.Errorf("env file %s: %v", shape.file, err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("env file %s: took %s, the wait should settle at once", shape.file, elapsed)
		}
	}
}

func TestWaitForWorktreeReady_keepsWaitingWhenTheFrameworksEnvFileIsMissing(t *testing.T) {
	wt := registerFrameworkWorktree(t, "wp-config.php", "php-const")
	// A stray .env must not pass for the file this framework is seeded with.
	writeFileTree(t, wt, ".env")

	if err := WaitForWorktreeReady(wt, 1*time.Second); err == nil {
		t.Error("want a timeout while the framework's env file is still missing")
	}
}
