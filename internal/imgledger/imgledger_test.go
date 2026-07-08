package imgledger

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempLedger points the ledger at a throwaway file for the test.
func withTempLedger(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pulled-images.json")
	pathFn = func() string { return path }
	t.Cleanup(func() { pathFn = defaultPath })
	return path
}

func TestRecordAndLoad(t *testing.T) {
	withTempLedger(t)

	if got := Load(); len(got) != 0 {
		t.Fatalf("fresh ledger should be empty, got %v", got)
	}

	Record("docker.io/library/mysql:8.4")
	Record("docker.io/library/redis:7")
	Record("docker.io/library/mysql:8.4") // duplicate is a no-op

	got := Load()
	if len(got) != 2 || !got["docker.io/library/mysql:8.4"] || !got["docker.io/library/redis:7"] {
		t.Fatalf("want the two distinct refs, got %v", got)
	}
}

// An empty ref is ignored, and a missing ledger file loads as empty rather than
// erroring, so cleanup stays conservative when nothing has been recorded.
func TestRecordEmptyAndMissingFile(t *testing.T) {
	path := withTempLedger(t)

	Record("")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("recording an empty ref must not create the ledger file")
	}
	if got := Load(); len(got) != 0 {
		t.Errorf("missing ledger should load empty, got %v", got)
	}
}

// A corrupt ledger loads as empty instead of propagating a parse error.
func TestLoadCorruptFile(t *testing.T) {
	path := withTempLedger(t)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); len(got) != 0 {
		t.Errorf("corrupt ledger should load empty, got %v", got)
	}
}
