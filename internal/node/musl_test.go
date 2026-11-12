package node

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type tarEntry struct {
	name, body, link string
	dir              bool
}

func tarball(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755}
		switch {
		case e.dir:
			hdr.Typeflag = tar.TypeDir
		case e.link != "":
			hdr.Typeflag, hdr.Linkname = tar.TypeSymlink, e.link
		default:
			hdr.Typeflag, hdr.Size = tar.TypeReg, int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func writeArchive(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "node.tar.gz")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMuslArch(t *testing.T) {
	for in, want := range map[string]string{"amd64": "x64", "arm64": "arm64"} {
		if got, err := muslArch(in); err != nil || got != want {
			t.Errorf("muslArch(%s) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := muslArch("riscv64"); err == nil {
		t.Error("an architecture Node builds no musl release for must fail")
	}
}

func TestExtractTree_DropsTheTopFolderAndKeepsLinks(t *testing.T) {
	archive := writeArchive(t, tarball(t, []tarEntry{
		{name: "node-v22.1.0-linux-x64-musl/", dir: true},
		{name: "node-v22.1.0-linux-x64-musl/bin/node", body: "NODE"},
		{name: "node-v22.1.0-linux-x64-musl/lib/node_modules/npm/bin/npm-cli.js", body: "NPM"},
		{name: "node-v22.1.0-linux-x64-musl/bin/npm", link: "../lib/node_modules/npm/bin/npm-cli.js"},
	}))
	dest := t.TempDir()
	if err := extractTree(archive, dest, true); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "bin", "node")); err != nil || string(b) != "NODE" {
		t.Errorf("bin/node = %q, %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "bin", "npm")); err != nil || string(b) != "NPM" {
		t.Errorf("bin/npm through its link = %q, %v", b, err)
	}
}

func TestExtractTree_RefusesToLeaveItsFolder(t *testing.T) {
	for name, entry := range map[string]tarEntry{
		"path":          {name: "node/../../escape", body: "x"},
		"absolute link": {name: "node/bin/evil", link: "/etc/passwd"},
		"relative link": {name: "node/bin/evil", link: "../../../etc/passwd"},
	} {
		archive := writeArchive(t, tarball(t, []tarEntry{entry}))
		if err := extractTree(archive, t.TempDir(), true); err == nil {
			t.Errorf("%s: an entry outside the archive's folder must be refused", name)
		}
	}
}

func TestMuslBinFor_PicksTheNewestMatchingBuild(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, v := range []string{"v22.9.0", "v22.23.3", "v22.10.1", "v24.1.0", "v2.0.0", "v22.99.0-1.tmp"} {
		makeBuild(t, v)
	}
	for spec, want := range map[string]string{"22": "v22.23.3", "v22.10": "v22.10.1", "24.1.0": "v24.1.0", "2": "v2.0.0", "20": "", "": ""} {
		got := MuslBinFor("/site", spec)
		if want == "" {
			if got != "" {
				t.Errorf("MuslBinFor(%q) = %q, want none", spec, got)
			}
			continue
		}
		if got != filepath.Join(MuslRoot(), want, "bin") {
			t.Errorf("MuslBinFor(%q) = %q, want %s", spec, got, want)
		}
	}
}

// The build is fetched once, checked against the release's own checksums, and
// reused after that without the network.
func TestMuslBin_DownloadsVerifiesAndReuses(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	arch, err := muslArch(runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	name := fmt.Sprintf("node-v22.1.0-linux-%s-musl.tar.gz", arch)
	data := tarball(t, []tarEntry{{name: "node-v22.1.0/bin/node", body: "NODE"}})
	sum := sha256.Sum256(data)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/v22.1.0/SHASUMS256.txt":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		case "/v22.1.0/" + name:
			w.Write(data) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	prev := muslRelease
	muslRelease = srv.URL
	t.Cleanup(func() { muslRelease = prev })

	bin, err := MuslBin(context.Background(), "v22.1.0", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(bin, "node")); string(b) != "NODE" {
		t.Errorf("node = %q", b)
	}
	before := hits
	if again, err := MuslBin(context.Background(), "v22.1.0", io.Discard); err != nil || again != bin || hits != before {
		t.Errorf("second call = %q, %v after %d more requests; want the cached build", again, err, hits-before)
	}
	if left, _ := filepath.Glob(filepath.Join(MuslRoot(), "*")); len(left) != 1 {
		t.Errorf("leftovers beside the build: %q", left)
	}
}

func TestMuslBin_RefusesATamperedArchive(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	arch, err := muslArch(runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	name := fmt.Sprintf("node-v22.1.0-linux-%s-musl.tar.gz", arch)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") {
			fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), name)
			return
		}
		w.Write(tarball(t, []tarEntry{{name: "n/bin/node", body: "EVIL"}})) //nolint:errcheck
	}))
	defer srv.Close()
	prev := muslRelease
	muslRelease = srv.URL
	t.Cleanup(func() { muslRelease = prev })

	if _, err := MuslBin(context.Background(), "v22.1.0", io.Discard); err == nil {
		t.Fatal("an archive that does not match its checksum must be refused")
	}
	if MuslBinFor("/site", "22") != "" {
		t.Error("a refused archive must leave no build behind")
	}
}

func TestMuslBin_RefusesAnInexactVersion(t *testing.T) {
	if _, err := MuslBin(context.Background(), "22", io.Discard); err == nil {
		t.Error("a major alone names no release to download")
	}
}

func makeBuild(t *testing.T, v string) {
	t.Helper()
	bin := filepath.Join(MuslRoot(), v, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
}

// A spec such as "lts/iron" names no version, and a host moved back to an older
// patch must not get a newer cached one: the shims use what lerd shell saw.
func TestMuslBinFor_FollowsTheRememberedVersion(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	makeBuild(t, "v20.19.0")
	makeBuild(t, "v22.23.3")
	makeBuild(t, "v22.24.0")
	for spec, want := range map[string]string{"lts/iron": "v20.19.0", "22": "v22.23.3"} {
		if err := RememberSpec("/site", spec, want); err != nil {
			t.Fatal(err)
		}
		if got := MuslBinFor("/site", spec); got != filepath.Join(MuslRoot(), want, "bin") {
			t.Errorf("MuslBinFor(%q) = %q, want %s", spec, got, want)
		}
	}
}

// Two first shells for one version each finish with the build in place rather
// than deleting each other's downloads.
func TestMuslBin_ConcurrentFirstCallsBothSucceed(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	arch, err := muslArch(runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	name := fmt.Sprintf("node-v22.1.0-linux-%s-musl.tar.gz", arch)
	data := tarball(t, []tarEntry{{name: "node-v22.1.0/bin/node", body: "NODE"}})
	sum := sha256.Sum256(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHASUMS256.txt") {
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
			return
		}
		w.Write(data) //nolint:errcheck
	}))
	defer srv.Close()
	prev := muslRelease
	muslRelease = srv.URL
	t.Cleanup(func() { muslRelease = prev })

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := MuslBin(context.Background(), "v22.1.0", io.Discard); errs <- err }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("a concurrent first call failed: %v", err)
		}
	}
}

// Two sites with the same spec can resolve to different versions through their
// own mise.toml; a shell in one must not move the other.
func TestMuslBinFor_KeepsEachSitesVersion(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	makeBuild(t, "v20.19.0")
	makeBuild(t, "v22.23.3")
	if err := RememberSpec("/a", "default", "v20.19.0"); err != nil {
		t.Fatal(err)
	}
	if err := RememberSpec("/b", "default", "v22.23.3"); err != nil {
		t.Fatal(err)
	}
	if got := MuslBinFor("/a", "default"); got != filepath.Join(MuslRoot(), "v20.19.0", "bin") {
		t.Errorf("site a = %q, want its own v20.19.0", got)
	}
}
