package php

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// stubOPcache records the containers a flush reached, on a host where FPM does
// or does not revalidate files by itself.
func stubOPcache(t *testing.T, manual bool) *[]string {
	t.Helper()
	var flushed []string
	origManual, origFlush := opcacheIsManual, flushOPcacheFn
	t.Cleanup(func() { opcacheIsManual, flushOPcacheFn = origManual, origFlush })
	opcacheIsManual = func() bool { return manual }
	flushOPcacheFn = func(c, mode string) error { flushed = append(flushed, c+":"+mode); return nil }
	return &flushed
}

func addTestSite(t *testing.T, s config.Site) string {
	t.Helper()
	s.Path = filepath.Join(tempRoot(t), s.Name)
	if err := os.MkdirAll(filepath.Join(s.Path, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(s); err != nil {
		t.Fatal(err)
	}
	return s.Path
}

func TestFlushOPcacheForDirReachesTheSitesFPM(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	flushed := stubOPcache(t, true)

	shared := addTestSite(t, config.Site{Name: "shop", PHPVersion: "8.4"})
	own := addTestSite(t, config.Site{Name: "app", PHPVersion: "8.3", Runtime: "fpm-custom"})
	FlushOPcacheForDir(filepath.Join(shared, "app"), podman.OPcacheApp)
	FlushOPcacheForDir(own, podman.OPcacheReset)

	if want := []string{"lerd-php84-fpm:app", "lerd-cfpm-app:reset"}; len(*flushed) != 2 || (*flushed)[0] != want[0] || (*flushed)[1] != want[1] {
		t.Errorf("flushed %q, want %q", *flushed, want)
	}
}

// A custom container runs a process tree lerd did not write, and an unlinked
// folder belongs to no site, so neither gets a signal.
func TestFlushOPcacheForDirLeavesOthersAlone(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	flushed := stubOPcache(t, true)

	custom := addTestSite(t, config.Site{Name: "node", PHPVersion: "8.4", ContainerPort: 3000})
	FlushOPcacheForDir(custom, podman.OPcacheApp)
	FlushOPcacheForDir(t.TempDir(), podman.OPcacheApp)

	if len(*flushed) != 0 {
		t.Errorf("flushed %q, want nothing", *flushed)
	}
}

func TestFlushOPcacheForDirIsANoOpWhereFPMRevalidates(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	flushed := stubOPcache(t, false)

	FlushOPcacheForDir(addTestSite(t, config.Site{Name: "shop", PHPVersion: "8.4"}), podman.OPcacheApp)
	if len(*flushed) != 0 {
		t.Errorf("flushed %q on a host that revalidates", *flushed)
	}
}
