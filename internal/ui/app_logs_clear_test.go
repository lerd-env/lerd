package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestClearAppLogs(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "storage", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, size int) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(logsDir, name), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("laravel.log", 100)
	write("laravel-2026-06-01.log", 250)
	// A non-log file in the same dir must be left alone (it isn't matched by
	// the *.log glob, so the reclaim can't touch it).
	if err := os.WriteFile(filepath.Join(logsDir, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sources := []config.FrameworkLogSource{{Path: "storage/logs/*.log", Format: "monolog"}}

	files, bytes, err := clearAppLogs(dir, sources)
	if err != nil {
		t.Fatalf("clearAppLogs: %v", err)
	}
	if files != 2 {
		t.Errorf("filesCleared: got %d want 2", files)
	}
	if bytes != 350 {
		t.Errorf("bytesCleared: got %d want 350", bytes)
	}
	if _, err := os.Stat(filepath.Join(logsDir, "laravel.log")); !os.IsNotExist(err) {
		t.Error("laravel.log should be deleted")
	}
	if _, err := os.Stat(filepath.Join(logsDir, ".gitignore")); err != nil {
		t.Error(".gitignore should be untouched by the log sweep")
	}

	// A second sweep over the now-empty set is a clean no-op.
	files, bytes, err = clearAppLogs(dir, sources)
	if err != nil || files != 0 || bytes != 0 {
		t.Errorf("empty sweep: got files=%d bytes=%d err=%v, want 0/0/nil", files, bytes, err)
	}
}
