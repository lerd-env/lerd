package filelock

import (
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestExclusiveBlocksOtherHandles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.lock")
	a, b := open(t, path), open(t, path)
	if err := TryExclusive(a); err != nil {
		t.Fatalf("first exclusive: %v", err)
	}
	if err := TryExclusive(b); err == nil {
		t.Fatal("second exclusive should fail while held")
	}
	if err := TryShared(b); err == nil {
		t.Fatal("shared should fail while exclusive held")
	}
	if err := Unlock(a); err != nil {
		t.Fatal(err)
	}
	if err := TryExclusive(b); err != nil {
		t.Fatalf("exclusive after unlock: %v", err)
	}
}
