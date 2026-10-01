package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A directory parked while the watcher runs has to be watched too, without a
// restart: lerd park only adds it to the config the watcher already read.
func TestWatch_PicksUpADirectoryParkedWhileRunning(t *testing.T) {
	prev := parkedReconcileEvery
	parkedReconcileEvery = 50 * time.Millisecond
	t.Cleanup(func() { parkedReconcileEvery = prev })

	first, later := t.TempDir(), t.TempDir()
	var mu sync.Mutex
	parked := []string{first}
	dirs := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), parked...)
	}

	found := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Watch(ctx, dirs, func(p string) { found <- p }, func(string) {}) }()

	mu.Lock()
	parked = append(parked, later)
	mu.Unlock()
	time.Sleep(200 * time.Millisecond)

	project := filepath.Join(later, "fresh")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-found:
		if got != project {
			t.Errorf("registered %q, want %q", got, project)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a project in a directory parked after start was never registered")
	}
}
