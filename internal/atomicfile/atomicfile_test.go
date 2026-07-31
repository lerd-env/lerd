package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteIfChanged_createsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	wrote, err := WriteIfChanged(path, []byte(`{"a":1}`), 0644)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !wrote {
		t.Error("a missing file must be written")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("content = %q, want %q", got, `{"a":1}`)
	}
}

// The reason this package exists: a daemon re-persisting an unchanged snapshot
// on a fixed tick must not touch the disk at all, so an idle laptop isn't woken
// every tick to record nothing.
func TestWriteIfChanged_identicalContentLeavesFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	data := []byte("[]")
	if _, err := WriteIfChanged(path, data, 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Backdate so any rewrite moves mtime forward detectably regardless of
	// filesystem timestamp granularity.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	wrote, err := WriteIfChanged(path, data, 0644)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if wrote {
		t.Error("identical content must not be written")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !after.ModTime().Equal(old) {
		t.Errorf("mtime moved from %v to %v; the file was rewritten", old, after.ModTime())
	}
}

func TestWriteIfChanged_rewritesChangedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	if _, err := WriteIfChanged(path, []byte("[]"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	wrote, err := WriteIfChanged(path, []byte(`[{"site":"a"}]`), 0644)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !wrote {
		t.Error("changed content must be written")
	}
	got, _ := os.ReadFile(path)
	if string(got) != `[{"site":"a"}]` {
		t.Errorf("content = %q, want the new bytes", got)
	}
}

func TestWriteIfChanged_createsParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "snap.json")
	if _, err := WriteIfChanged(path, []byte("[]"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created under a missing parent: %v", err)
	}
}

func TestWriteIfChanged_leavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	if _, err := WriteIfChanged(path, []byte("[]"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the target file, got %d entries", len(entries))
	}
}
