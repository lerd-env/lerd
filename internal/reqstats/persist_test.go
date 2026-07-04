package reqstats

import (
	"path/filepath"
	"testing"
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

func TestLoadMissingFile(t *testing.T) {
	if got := Load(filepath.Join(t.TempDir(), "nope.json")); got != nil {
		t.Errorf("missing file = %v, want nil", got)
	}
	if _, ok := LoadSite(filepath.Join(t.TempDir(), "nope.json"), "x"); ok {
		t.Error("missing file must yield ok=false")
	}
}
