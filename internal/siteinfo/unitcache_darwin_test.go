//go:build darwin

package siteinfo

import (
	"os"
	"path/filepath"
	"testing"
)

// A launchd job exposes no WorkingDirectory, so darwin recovers it from the
// guard script lerd writes for the worker. Without it every unit resolves with
// an empty probe path, which is what left orphaned per-worktree units invisible
// to the pruner on macOS.
func TestAllUnitMeta_darwinReadsWorkerWorkingDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	dir := filepath.Join(tmp, "lerd", "run", "workers")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	script := "cd '" + tmp + "/app-feat-login' || exit 1\nexec /bin/sh -c 'npm run dev'\n"
	if err := os.WriteFile(filepath.Join(dir, "lerd-vite-app-feat-login.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	InvalidateUnitCache()
	t.Cleanup(InvalidateUnitCache)

	meta := AllUnitMeta()
	want := tmp + "/app-feat-login"
	for _, unit := range []string{"lerd-vite-app-feat-login", "lerd-vite-app-feat-login.service"} {
		if meta[unit].WorkingDir != want {
			t.Errorf("AllUnitMeta()[%q].WorkingDir = %q, want %q", unit, meta[unit].WorkingDir, want)
		}
	}
}

func TestAllUnitMeta_darwinEmptyWithoutGuardScripts(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	InvalidateUnitCache()
	t.Cleanup(InvalidateUnitCache)

	if meta := AllUnitMeta(); len(meta) != 0 {
		t.Errorf("AllUnitMeta = %v, want empty when no worker guard scripts exist", meta)
	}
}
