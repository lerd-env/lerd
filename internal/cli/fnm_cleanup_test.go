package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Once another manager drives Node, lerd's own fnm is a binary nothing runs, so
// it goes rather than sitting in the bin dir being reported as missing.
func TestRemoveFnmBinary(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := config.BinDir()
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fnm := filepath.Join(bin, "fnm")
	for _, f := range []string{fnm, fnm + ".version", filepath.Join(bin, "mkcert")} {
		if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	removeFnmBinary()

	if _, err := os.Stat(fnm); !os.IsNotExist(err) {
		t.Errorf("fnm still present: %v", err)
	}
	if _, err := os.Stat(fnm + ".version"); !os.IsNotExist(err) {
		t.Errorf("fnm stamp still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bin, "mkcert")); err != nil {
		t.Errorf("mkcert should be untouched: %v", err)
	}
}

// A bin dir without fnm is the normal case on a fresh mise install, and the
// cleanup has to be a no-op there rather than an error.
func TestRemoveFnmBinary_NoFnm(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	removeFnmBinary()
}
