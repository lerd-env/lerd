package nativephp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// SPX is configured by an ini of its own, and a directory the pool does not
// scan is a directory it never reads: spx.http_enabled stayed unset, SPX never
// intercepted, and the profiler dashboard answered with an empty page.
func TestIniScanDirsIncludesSPX(t *testing.T) {
	dirs := IniScanDirs("8.4")
	want := filepath.Dir(config.SpxIniFile())
	var found, overrideAt, spxAt = false, -1, -1
	for i, d := range dirs {
		if d == want {
			found, spxAt = true, i
		}
		if d == OverrideDir("8.4") {
			overrideAt = i
		}
	}
	if !found {
		t.Fatalf("scan dirs %v do not include SPX's own ini directory %s", dirs, want)
	}
	// The override rewrites SPX's container paths, so it has to be scanned
	// after the file it is correcting.
	if spxAt > overrideAt {
		t.Errorf("SPX ini at %d is scanned after the override at %d", spxAt, overrideAt)
	}
}

// spx.data_dir names a path that exists only inside the image, and SPX writes
// its profiles there.
func TestOverrideRewritesTheSPXDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	got := overrideIni("8.4")
	if !strings.Contains(got, "spx.data_dir="+config.SpxDataDir()) {
		t.Errorf("override should point SPX at its host directory:\n%s", got)
	}
	if strings.Contains(got, "spx.data_dir=/var/spx") {
		t.Errorf("override must not keep the image path:\n%s", got)
	}
}
