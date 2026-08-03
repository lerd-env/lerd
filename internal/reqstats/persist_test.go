package reqstats

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)
	recordN(a, "myapp.test", "GET", "/reports/7", 380, 10)

	path := filepath.Join(t.TempDir(), "request-stats.json")
	if err := a.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok := LoadSite(path, "myapp")
	if !ok {
		t.Fatal("expected loaded site")
	}
	if len(got.Slow) == 0 || got.Slow[0].Route != "GET /reports/:id" {
		t.Errorf("loaded slow routes = %+v", got.Slow)
	}
	if got.MedianMillis < 40 || got.MedianMillis > 60 {
		t.Errorf("loaded median = %v", got.MedianMillis)
	}
}

// The watcher saves on a fixed tick for the daemon's life, so an unchanged
// snapshot must not reach the disk at all.
func TestSaveSnapshot_unchangedSnapshotLeavesFileUntouched(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)

	path := filepath.Join(t.TempDir(), "request-stats.json")
	if err := a.Save(path); err != nil {
		t.Fatalf("seed: %v", err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := a.Save(path); err != nil {
		t.Fatalf("resave: %v", err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !st.ModTime().Equal(old) {
		t.Error("re-saving an unchanged snapshot rewrote the file")
	}
}

func TestSaveSnapshot_changedSnapshotIsWritten(t *testing.T) {
	a := New(siteResolver(map[string]string{"myapp.test": "myapp"}))
	recordN(a, "myapp.test", "GET", "/home", 40, 20)

	path := filepath.Join(t.TempDir(), "request-stats.json")
	if err := a.Save(path); err != nil {
		t.Fatalf("seed: %v", err)
	}

	recordN(a, "myapp.test", "GET", "/reports/7", 380, 10)
	if err := a.Save(path); err != nil {
		t.Fatalf("resave: %v", err)
	}

	got, ok := LoadSite(path, "myapp")
	if !ok || len(got.Slow) == 0 {
		t.Error("a changed snapshot must be persisted")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if got := Load(filepath.Join(t.TempDir(), "nope.json")); got != nil {
		t.Errorf("missing file = %v, want nil", got)
	}
	if _, ok := LoadSite(filepath.Join(t.TempDir(), "nope.json"), "x"); ok {
		t.Error("missing file must yield ok=false")
	}
}
