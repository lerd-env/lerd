package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A partial framework overlay must stay additive: starting one of its workers
// in a linked project must never rewrite that project's .lerd.yaml (#1910).
func TestRunWorkerStart_partialOverlayLeavesProjectConfigAlone(t *testing.T) {
	cases := map[string]struct{ worker, overlay string }{
		"missing program": {"pulse", "name: laravel\nversion: \"10\"\nworkers:\n  pulse:\n    label: Pulse\n    command: vendor/bin/not-installed pulse:check\n"},
		"bad restart":     {"pulse", "name: laravel\nversion: \"10\"\nworkers:\n  pulse:\n    command: php artisan pulse:check\n    restart: sometimes\n"},
		"bad schedule":    {"nightly", "name: laravel\nversion: \"10\"\nworkers:\n  nightly:\n    command: php artisan inspire\n    schedule: \"every blue moon\"\n    conflicts_with: [queue]\n"},
		"override queue":  {"queue", "name: laravel\nversion: \"10\"\nworkers:\n  queue:\n    command: php artisan queue:work\n    schedule: \"every blue moon\"\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
			swapMgr(t, &stopTrackingMgr{})
			prevCal := calendarValidFn
			calendarValidFn = func(string) bool { return false }
			t.Cleanup(func() { calendarValidFn = prevCal })

			sitePath := filepath.Join(tmp, "acme")
			os.MkdirAll(sitePath, 0755)
			lock := `{"packages":[{"name":"laravel/framework","version":"v10.48.4"}]}`
			os.WriteFile(filepath.Join(sitePath, "composer.lock"), []byte(lock), 0644)
			os.WriteFile(filepath.Join(sitePath, "composer.json"), []byte(`{"require":{"laravel/framework":"^10.0"}}`), 0644)
			os.WriteFile(filepath.Join(sitePath, "artisan"), []byte("<?php\n"), 0644)
			original := "domains:\n    - acme\nphp_version: \"8.2\"\nframework: laravel\nframework_version: \"10\"\nservices:\n    - mysql\n    - redis\n"
			lerdYAML := filepath.Join(sitePath, ".lerd.yaml")
			os.WriteFile(lerdYAML, []byte(original), 0644)

			if err := config.AddSite(config.Site{
				Name: "acme", Path: sitePath, Domains: []string{"acme.test"},
				PHPVersion: "8.2", Framework: "laravel",
			}); err != nil {
				t.Fatal(err)
			}
			os.MkdirAll(config.FrameworksDir(), 0755)
			os.WriteFile(filepath.Join(config.FrameworksDir(), "laravel.yaml"), []byte(tc.overlay), 0644)

			t.Chdir(sitePath)
			err := runWorkerStart(tc.worker, nil)
			t.Logf("runWorkerStart: %v", err)

			got, _ := os.ReadFile(lerdYAML)
			if string(got) != original {
				t.Fatalf(".lerd.yaml was rewritten:\n--- before\n%s--- after\n%s", original, got)
			}
		})
	}
}
