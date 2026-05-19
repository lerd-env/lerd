package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncNodeGlobalBins_CreatesWrappers(t *testing.T) {
	root := t.TempDir()
	sourceBin := filepath.Join(root, "node-global", "bin")
	targetBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBin, "pm2"), []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncNodeGlobalBins(sourceBin, targetBin, "/fake/fnm"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	wrapper := filepath.Join(targetBin, "pm2")
	data, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatalf("expected wrapper at %s: %v", wrapper, err)
	}
	body := string(data)
	if !strings.Contains(body, nodeShimMarker) {
		t.Errorf("wrapper missing marker: %q", body)
	}
	if !strings.Contains(body, filepath.Join(sourceBin, "pm2")) {
		t.Errorf("wrapper missing real bin path: %q", body)
	}
	if !strings.Contains(body, "/fake/fnm") {
		t.Errorf("wrapper missing fnm path: %q", body)
	}
	if !strings.Contains(body, "--using=default") {
		t.Errorf("wrapper missing --using=default flag: %q", body)
	}
	info, err := os.Stat(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("wrapper not executable: mode=%v", info.Mode())
	}
}

func TestSyncNodeGlobalBins_RemovesOrphans(t *testing.T) {
	root := t.TempDir()
	sourceBin := filepath.Join(root, "node-global", "bin")
	targetBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetBin, 0o755); err != nil {
		t.Fatal(err)
	}
	// stale wrapper with marker, source no longer present
	orphan := filepath.Join(targetBin, "vite")
	content := "#!/bin/sh\n# " + nodeShimMarker + "\nexec /old/path\n"
	if err := os.WriteFile(orphan, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncNodeGlobalBins(sourceBin, targetBin, "/fake/fnm"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan removed, got err=%v", err)
	}
}

func TestSyncNodeGlobalBins_PreservesForeignFiles(t *testing.T) {
	root := t.TempDir()
	sourceBin := filepath.Join(root, "node-global", "bin")
	targetBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetBin, 0o755); err != nil {
		t.Fatal(err)
	}
	// user-installed binary without lerd's marker
	user := filepath.Join(targetBin, "pm2")
	if err := os.WriteFile(user, []byte("#!/bin/sh\necho hi from user\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// new global of the same name appears in source
	if err := os.WriteFile(filepath.Join(sourceBin, "pm2"), []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncNodeGlobalBins(sourceBin, targetBin, "/fake/fnm"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	data, err := os.ReadFile(user)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hi from user") {
		t.Errorf("user file was clobbered: %q", string(data))
	}
}

func TestSyncNodeGlobalBins_IgnoresBinariesContainingMarker(t *testing.T) {
	// Regression: the marker is a Go string constant, so the lerd binary
	// itself contains the marker bytes. If sync ever scans binaries the
	// same way as shell wrappers, it will delete lerd from ~/.local/bin/.
	root := t.TempDir()
	sourceBin := filepath.Join(root, "node-global", "bin")
	targetBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(sourceBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetBin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Fake binary: ELF-ish magic followed by the marker substring as data.
	fakeBinary := append([]byte{0x7f, 'E', 'L', 'F', 0, 0, 0, 0}, []byte("lerd-managed npm global shim")...)
	binPath := filepath.Join(targetBin, "lerd")
	if err := os.WriteFile(binPath, fakeBinary, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncNodeGlobalBins(sourceBin, targetBin, "/fake/fnm"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("native binary was wrongly removed: %v", err)
	}
}

func TestSyncNodeGlobalBins_MissingSourceIsNoOp(t *testing.T) {
	root := t.TempDir()
	sourceBin := filepath.Join(root, "node-global", "bin")
	targetBin := filepath.Join(root, "local-bin")
	if err := os.MkdirAll(targetBin, 0o755); err != nil {
		t.Fatal(err)
	}
	// pre-existing orphan should still be cleaned up even when source is absent
	orphan := filepath.Join(targetBin, "ghost")
	content := "#!/bin/sh\n# " + nodeShimMarker + "\n"
	if err := os.WriteFile(orphan, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := syncNodeGlobalBins(sourceBin, targetBin, "/fake/fnm"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan removed when source missing, err=%v", err)
	}
}
