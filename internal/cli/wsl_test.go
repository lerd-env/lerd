package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// lerd ships its own mkcert under BinDir and never puts it on PATH, so the CA
// has to be found through that copy or wsl:setup skips the Windows trust step
// on every normal install.
func TestMkcertRootCAPEM_UsesLerdsOwnMkcert(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("PATH", "")

	caroot := filepath.Join(data, "caroot")
	if err := os.MkdirAll(caroot, 0o755); err != nil {
		t.Fatal(err)
	}
	pem := filepath.Join(caroot, "rootCA.pem")
	if err := os.WriteFile(pem, []byte("pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(data, "lerd", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho " + caroot + "\n"
	if err := os.WriteFile(filepath.Join(bin, "mkcert"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := mkcertRootCAPEM()
	if !ok || got != pem {
		t.Fatalf("mkcertRootCAPEM() = %q, %v; want %q, true", got, ok, pem)
	}
}

func TestMkcertRootCAPEM_NoCAYet(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", "")
	if got, ok := mkcertRootCAPEM(); ok {
		t.Fatalf("expected no CA without mkcert, got %q", got)
	}
}
