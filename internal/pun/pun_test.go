package pun

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/tools"
)

// The release tarball nests the binary in a folder named after the target.
func fakeRelease(t *testing.T, calls *int) {
	t.Helper()
	prev := fetch
	t.Cleanup(func() { fetch = prev })
	fetch = func(_ context.Context, _, dest string, _ os.FileMode, digest string, _ io.Writer) error {
		*calls++
		if len(digest) != 64 {
			t.Errorf("digest = %q, want the pinned sha256", digest)
		}
		f, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer f.Close()
		gz := gzip.NewWriter(f)
		tw := tar.NewWriter(gz)
		body := []byte("#!/bin/sh\necho pun\n")
		tw.WriteHeader(&tar.Header{Name: "punphp-x86_64-unknown-linux-musl/README.md", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})          //nolint:errcheck
		tw.Write([]byte("hi"))                                                                                                                //nolint:errcheck
		tw.WriteHeader(&tar.Header{Name: "punphp-x86_64-unknown-linux-musl/pun", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}) //nolint:errcheck
		tw.Write(body)                                                                                                                        //nolint:errcheck
		tw.Close()
		return gz.Close()
	}
}

func TestEnsureInstallsThePinOnceAndStampsIt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	calls := 0
	fakeRelease(t, &calls)

	bin, err := Ensure(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if info, err := os.Stat(bin); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("pun not installed executable at %s: %v", bin, err)
	}
	if got, want := tools.InstalledVersion("pun"), tools.Load(context.Background()).Tools["pun"].Version; got != want {
		t.Errorf("stamp = %q, want %q", got, want)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(bin), "*.tar.gz")); len(left) > 0 {
		t.Error("the downloaded archive was left behind")
	}
	if _, err := Ensure(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("downloaded %d times, want once", calls)
	}
}
