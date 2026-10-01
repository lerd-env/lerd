package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// siteWithTunableQueue seeds a store framework whose queue worker declares a
// tune_command, registers a site for it, and returns the site's path.
func siteWithTunableQueue(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	storeDir := config.StoreFrameworksDir()
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "name: tinyfw\nlabel: TinyFW\nworkers:\n  queue:\n    label: Queue Worker\n" +
		"    command: php artisan queue:work --queue=default --tries=3\n" +
		"    tune_command: php artisan queue:work --queue={queue} --tries={tries}\n"
	if err := os.WriteFile(filepath.Join(storeDir, "tinyfw.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: tinyfw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		{Name: "shop", Path: dir, Framework: "tinyfw"},
	}}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestApplyWorkerOptions(t *testing.T) {
	t.Run("keeps only what the project changed", func(t *testing.T) {
		dir := siteWithTunableQueue(t)
		err := ApplyWorkerOptions("shop", dir, "8.4", "queue", map[string]string{
			"queue": "high,default,low",
			"tries": "3", // the definition's own default
		})
		if err != nil {
			t.Fatalf("ApplyWorkerOptions: %v", err)
		}
		got := config.ProjectWorkerOptions(dir, "queue")
		if got["queue"] != "high,default,low" {
			t.Errorf("queue: got %q, want high,default,low", got["queue"])
		}
		if _, ok := got["tries"]; ok {
			t.Errorf("tries equals the definition default and should not be persisted: %v", got)
		}
	})

	t.Run("clearing a value goes back to the definition", func(t *testing.T) {
		dir := siteWithTunableQueue(t)
		if err := ApplyWorkerOptions("shop", dir, "8.4", "queue", map[string]string{"queue": "emails"}); err != nil {
			t.Fatal(err)
		}
		if err := ApplyWorkerOptions("shop", dir, "8.4", "queue", map[string]string{"queue": ""}); err != nil {
			t.Fatal(err)
		}
		if got := config.ProjectWorkerOptions(dir, "queue"); len(got) != 0 {
			t.Errorf("got %v, want nothing persisted", got)
		}
	})

	t.Run("a value that would add an argument is refused", func(t *testing.T) {
		dir := siteWithTunableQueue(t)
		err := ApplyWorkerOptions("shop", dir, "8.4", "queue", map[string]string{"queue": "high --daemon"})
		if err == nil {
			t.Fatal("want an error for a value carrying whitespace")
		}
		if got := config.ProjectWorkerOptions(dir, "queue"); len(got) != 0 {
			t.Errorf("a refused value must not be persisted, got %v", got)
		}
	})

	t.Run("the resolved command is what the project asked for", func(t *testing.T) {
		dir := siteWithTunableQueue(t)
		if err := ApplyWorkerOptions("shop", dir, "8.4", "queue", map[string]string{"queue": "high,low"}); err != nil {
			t.Fatal(err)
		}
		fw, ok := config.GetFrameworkForDir("tinyfw", dir)
		if !ok {
			t.Fatal("framework not resolved")
		}
		want := "php artisan queue:work --queue=high,low --tries=3"
		if got := resolveWorkerCommand(dir, "queue", fw.Workers["queue"]); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// A start flag equal to the definition's default is not a choice the project
// made, so it stays out of .lerd.yaml and a later store change to the default
// still lands. Passing the default also takes back an earlier override.
func TestPersistWorkerOptions_dropsDefaults(t *testing.T) {
	dir := siteWithTunableQueue(t)
	fw, ok := config.GetFrameworkForDir("tinyfw", dir)
	if !ok {
		t.Fatal("the test definition did not resolve")
	}
	w := fw.Workers["queue"]

	if err := persistWorkerOptions(dir, "queue", w, map[string]string{"queue": "high", "tries": "5"}); err != nil {
		t.Fatal(err)
	}
	if err := persistWorkerOptions(dir, "queue", w, map[string]string{"tries": "3"}); err != nil {
		t.Fatal(err)
	}
	got := config.ProjectWorkerOptions(dir, "queue")
	if len(got) != 1 || got["queue"] != "high" {
		t.Errorf("stored options = %v, want only queue=high", got)
	}

	if err := persistWorkerOptions(dir, "queue", w, map[string]string{"queue": "default"}); err != nil {
		t.Fatal(err)
	}
	if got := config.ProjectWorkerOptions(dir, "queue"); len(got) != 0 {
		t.Errorf("stored options = %v, want none once every value is the default", got)
	}
}
