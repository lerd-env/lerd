package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestShimLeadingEnv_PrependsBinDirToPath(t *testing.T) {
	binDir := config.BinDir()
	sep := string(os.PathListSeparator)

	out := shimLeadingEnv([]string{"FOO=bar", "PATH=/usr/bin" + sep + "/bin", "BAZ=qux"})

	var path string
	for _, kv := range out {
		if name, val, ok := strings.Cut(kv, "="); ok && name == "PATH" {
			path = val
		}
	}
	want := binDir + sep + "/usr/bin" + sep + "/bin"
	if path != want {
		t.Errorf("PATH = %q, want %q", path, want)
	}
	if !strings.HasPrefix(path, binDir+sep) {
		t.Errorf("lerd bin dir must lead PATH so the php shim wins; got %q", path)
	}
	// non-PATH vars pass through untouched
	if !envHas(out, "FOO=bar") || !envHas(out, "BAZ=qux") {
		t.Errorf("non-PATH vars were not preserved: %v", out)
	}
}

func TestShimLeadingEnv_AddsPathWhenAbsent(t *testing.T) {
	out := shimLeadingEnv([]string{"FOO=bar"})
	if !envHas(out, "PATH="+config.BinDir()) {
		t.Errorf("expected PATH to be added with bin dir, got %v", out)
	}
}

func TestShimLeadingEnv_MatchesPathCaseInsensitively(t *testing.T) {
	sep := string(os.PathListSeparator)
	out := shimLeadingEnv([]string{"Path=/usr/bin"})
	// only one PATH-ish entry should exist, with the bin dir prepended
	count := 0
	for _, kv := range out {
		if name, _, ok := strings.Cut(kv, "="); ok && strings.EqualFold(name, "PATH") {
			count++
			if !strings.HasPrefix(kv, "Path="+config.BinDir()+sep) {
				t.Errorf("existing key casing not preserved or bin dir missing: %q", kv)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one PATH entry, got %d in %v", count, out)
	}
}

func envHas(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

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

// TestRunBun_FailureReturnsErrorWhenNotExiting guards the lerd link/setup
// regression where a failed `npm run production` / `bun run build` killed the
// whole process via os.Exit, bypassing the step loop's failure feedback. With
// exitOnFail false the non-zero child exit must come back as an error so the
// caller can report it, not terminate lerd.
func TestRunBun_FailureReturnsErrorWhenNotExiting(t *testing.T) {
	// /bin/sh stands in for the bun binary; the script exits non-zero.
	err := runBun(t.TempDir(), "/bin/sh", []string{"-c", "exit 3"}, false)
	if err == nil {
		t.Fatal("expected an error from a non-zero child exit, got nil")
	}
}

// TestRunBun_SuccessReturnsNil confirms the happy path still returns nil.
func TestRunBun_SuccessReturnsNil(t *testing.T) {
	if err := runBun(t.TempDir(), "/bin/sh", []string{"-c", "exit 0"}, false); err != nil {
		t.Fatalf("expected nil on success, got %v", err)
	}
}
