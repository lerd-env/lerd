package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func envSite(t *testing.T, env, backup string) config.Site {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{".env": env, ".env.before_lerd": backup} {
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return config.Site{Name: filepath.Base(dir), Path: dir}
}

func TestSitesWithEnvBackup(t *testing.T) {
	const lerdEnv, original = "DB_HOST=lerd-mysql\n", "DB_HOST=127.0.0.1\n"
	changed := envSite(t, lerdEnv, original)
	urlOnly := envSite(t, "APP_URL=http://a.test\n", "APP_URL=http://localhost\n")
	sites := []config.Site{
		changed,
		urlOnly,
		envSite(t, lerdEnv, ""),        // nothing saved
		envSite(t, original, original), // already restored by hand
		envSite(t, "", original),       // env file deleted
		{Name: "gone", Path: filepath.Join(t.TempDir(), "missing")},
	}
	got := sitesWithEnvBackup(sites)
	if want := []string{changed.Path, urlOnly.Path}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sitesWithEnvBackup = %v, want %v", got, want)
	}
}

// A project that cannot be restored must not stop the others.
func TestRestoreSiteEnvs(t *testing.T) {
	good := envSite(t, "DB_HOST=lerd-mysql\nNEW=1\n", "DB_HOST=127.0.0.1\n")
	restoreSiteEnvs([]string{filepath.Join(t.TempDir(), "missing"), good.Path})
	body, err := os.ReadFile(filepath.Join(good.Path, ".env"))
	if err != nil || string(body) != "DB_HOST=127.0.0.1\nNEW=1\n" {
		t.Fatalf(".env = %q, %v", body, err)
	}
}

func TestUninstallPlan_flagsNeverRestoreEnvs(t *testing.T) {
	for _, flags := range [][2]bool{{true, false}, {false, true}} {
		p, err := uninstallPlanFor(flags[0], flags[1])
		if err != nil || len(p.restoreEnvs) != 0 {
			t.Errorf("force=%v keepData=%v: restoreEnvs = %v, err %v", flags[0], flags[1], p.restoreEnvs, err)
		}
	}
}
