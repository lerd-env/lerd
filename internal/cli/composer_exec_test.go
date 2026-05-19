package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncComposerGlobalBins_CreatesWrappers(t *testing.T) {
	root := t.TempDir()
	sourceBin := filepath.Join(root, "composer", "vendor", "bin")
	targetBin := filepath.Join(root, "lerd-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBin, "psysh"), []byte("#!/usr/bin/env php\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncComposerGlobalBins(sourceBin, targetBin, "/fake/lerd"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(targetBin, "psysh"))
	if err != nil {
		t.Fatalf("expected wrapper: %v", err)
	}
	bs := string(body)
	if !strings.Contains(bs, composerShimMarker) {
		t.Errorf("wrapper missing marker: %q", bs)
	}
	if !strings.Contains(bs, "/fake/lerd") {
		t.Errorf("wrapper missing lerd path: %q", bs)
	}
	if !strings.Contains(bs, filepath.Join(sourceBin, "psysh")) {
		t.Errorf("wrapper missing real bin path: %q", bs)
	}
	if !strings.Contains(bs, "php") {
		t.Errorf("wrapper missing php command: %q", bs)
	}
}

func TestSyncComposerGlobalBins_IgnoresNodeWrappers(t *testing.T) {
	// Cross-category isolation: composer sync must not remove a node-managed
	// wrapper that happens to share targetBin, and vice versa, so the two
	// syncs can coexist in ~/.local/share/lerd/bin/.
	root := t.TempDir()
	sourceBin := filepath.Join(root, "composer", "vendor", "bin")
	targetBin := filepath.Join(root, "lerd-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetBin, 0o755); err != nil {
		t.Fatal(err)
	}
	nodeWrapper := filepath.Join(targetBin, "pm2")
	body := "#!/bin/sh\n# " + nodeShimMarker + "\nexec /fake/fnm exec --using=default -- /old/pm2\n"
	if err := os.WriteFile(nodeWrapper, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncComposerGlobalBins(sourceBin, targetBin, "/fake/lerd"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(nodeWrapper); err != nil {
		t.Fatalf("node wrapper was wrongly removed: %v", err)
	}
}

func TestSyncComposerGlobalBins_RemovesOrphans(t *testing.T) {
	root := t.TempDir()
	sourceBin := filepath.Join(root, "composer", "vendor", "bin")
	targetBin := filepath.Join(root, "lerd-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetBin, 0o755); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(targetBin, "psysh")
	body := "#!/bin/sh\n# " + composerShimMarker + "\nexec /old/lerd php /old/psysh\n"
	if err := os.WriteFile(orphan, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncComposerGlobalBins(sourceBin, targetBin, "/fake/lerd"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan removed, got err=%v", err)
	}
}

func TestComposerGlobalBinDir_UsesXDG(t *testing.T) {
	t.Setenv("COMPOSER_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	got := composerGlobalBinDir()
	want := "/tmp/xdg/composer/vendor/bin"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestComposerGlobalBinDir_RespectsComposerHome(t *testing.T) {
	t.Setenv("COMPOSER_HOME", "/custom/composer")
	got := composerGlobalBinDir()
	want := "/custom/composer/vendor/bin"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
