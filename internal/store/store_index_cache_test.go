package store

import (
	"os"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// RefreshIndex must persist the fetched index to config.StoreIndexFile() so the
// offline detection and listing paths can read the full catalogue without a
// network round trip. loadCachedIndex reads that same file back.
func TestRefreshIndex_CachesToDisk(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	srv := testServer(t)
	defer srv.Close()
	c := testClient(t, srv)

	if _, err := c.RefreshIndex(); err != nil {
		t.Fatalf("RefreshIndex: %v", err)
	}
	if _, err := os.Stat(config.StoreIndexFile()); err != nil {
		t.Fatalf("expected cached index at %s: %v", config.StoreIndexFile(), err)
	}

	idx, err := loadCachedIndex()
	if err != nil {
		t.Fatalf("loadCachedIndex: %v", err)
	}
	if len(idx.Frameworks) != 2 || idx.Frameworks[1].Name != "symfony" {
		t.Fatalf("unexpected cached index: %+v", idx)
	}
}
