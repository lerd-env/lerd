package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
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

// The bug this guards: writers to the same target run both in-process (the
// watcher tick) and out-of-process (`lerd unlink`). A shared, fixed temp name
// lets two of them truncate and fill the same file at offset 0, and the rename
// then publishes a merge of both payloads that no reader can parse.
func TestWriteIfChanged_concurrentWritersNeverPublishAMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")

	const writers = 8
	payloads := make([][]byte, writers)
	for i := range payloads {
		payloads[i] = bytes.Repeat([]byte{byte('a' + i)}, 256*1024)
	}

	var wg sync.WaitGroup
	for i := range payloads {
		wg.Add(1)
		go func(data []byte) {
			defer wg.Done()
			for n := 0; n < 5; n++ {
				if _, err := WriteIfChanged(path, data, 0644); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(payloads[i])
	}
	wg.Wait()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	matched := false
	for _, p := range payloads {
		if bytes.Equal(got, p) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("published content is a merge of concurrent writes (%d bytes, first byte %q, last byte %q)",
			len(got), got[0], got[len(got)-1])
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the target file after concurrent writes, got %d entries", len(entries))
	}
}

// A write that cannot complete must not leave its temp behind; here the rename
// fails because the target path is a directory.
func TestWriteIfChanged_removesItsTempWhenTheWriteFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := WriteIfChanged(path, []byte("[]"), 0644); err == nil {
		t.Fatal("expected an error when the target cannot be replaced")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("a failed write left its temp file behind: %d entries", len(entries))
	}
}

func TestWriteIfChanged_appliesTheRequestedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	if _, err := WriteIfChanged(path, []byte("[]"), 0640); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
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
