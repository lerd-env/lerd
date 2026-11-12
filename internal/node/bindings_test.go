package node

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLock(t *testing.T, dir, body string, installed ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range installed {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

const rollupLock = `{"lockfileVersion": 3, "packages": {
  "": {"name": "app"},
  "node_modules/rollup": {"version": "4.0.0"},
  "node_modules/@rollup/rollup-darwin-arm64": {"optional": true, "os": ["darwin"], "cpu": ["arm64"]},
  "node_modules/@rollup/rollup-linux-arm64-gnu": {"optional": true, "os": ["linux"], "cpu": ["arm64"], "libc": ["glibc"]},
  "node_modules/@rollup/rollup-linux-arm64-musl": {"optional": true, "os": ["linux"], "cpu": ["arm64"], "libc": ["musl"]},
  "node_modules/@rollup/rollup-linux-x64-musl": {"optional": true, "os": ["linux"], "cpu": ["x64"], "libc": ["musl"]},
  "node_modules/fsevents": {"optional": true, "os": ["darwin"]}
}}`

func missingPaths(bs []ContainerBinding) []string {
	var paths []string
	for _, b := range bs {
		paths = append(paths, b.Path)
	}
	return paths
}

func TestMissingContainerBindings(t *testing.T) {
	cases := []struct {
		name      string
		installed []string
		want      string
	}{
		{"installed on a Mac", []string{"node_modules/rollup", "node_modules/@rollup/rollup-darwin-arm64", "node_modules/fsevents"}, "node_modules/@rollup/rollup-linux-arm64-musl"},
		{"installed on a glibc host", []string{"node_modules/rollup", "node_modules/@rollup/rollup-linux-arm64-gnu"}, "node_modules/@rollup/rollup-linux-arm64-musl"},
		{"installed in the container", []string{"node_modules/rollup", "node_modules/@rollup/rollup-linux-arm64-musl"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeLock(t, dir, rollupLock, c.installed...)
			if got := strings.Join(missingPaths(missingContainerBindings(dir, "arm64")), ","); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// fsevents has no Linux build at all, so lacking it costs the container nothing.
func TestMissingContainerBindingsIgnoresHostOnlyPackages(t *testing.T) {
	dir := t.TempDir()
	writeLock(t, dir, `{"packages": {"node_modules/fsevents": {"optional": true, "os": ["darwin"]}}}`, "node_modules/fsevents")
	if got := missingContainerBindings(dir, "arm64"); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestMissingContainerBindingsNeedsInstalledModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(rollupLock), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := missingContainerBindings(dir, "arm64"); len(got) != 0 {
		t.Fatalf("no node_modules yet: got %v, want none", got)
	}
	if got := missingContainerBindings(t.TempDir(), "arm64"); len(got) != 0 {
		t.Fatalf("no lockfile: got %v, want none", got)
	}
}

// npmTarball is a package as the registry serves it, wrapped in package/.
func npmTarball(t *testing.T, name string) (data []byte, integrity string) {
	t.Helper()
	data = tarball(t, []tarEntry{
		{name: "package/package.json", body: `{"name": "` + name + `"}`},
		{name: "package/binding.node", body: "musl"},
	})
	sum := sha512.Sum512(data)
	return data, "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func registry(t *testing.T, files map[string][]byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// Two requirers can lock different versions of one binding, so each lands at
// its own lockfile path rather than in one shared folder.
func TestFetchContainerBindings_PlacesEachAtItsLockfilePath(t *testing.T) {
	top, topSum := npmTarball(t, "lightningcss-linux-arm64-musl@1.32.0")
	nested, nestedSum := npmTarball(t, "lightningcss-linux-arm64-musl@1.33.0")
	url := registry(t, map[string][]byte{"/top.tgz": top, "/nested.tgz": nested})
	dir := t.TempDir()
	writeLock(t, dir, `{"packages": {
	  "node_modules/lightningcss-linux-arm64-musl": {"os": ["linux"], "cpu": ["arm64"], "libc": ["musl"], "resolved": "`+url+`/top.tgz", "integrity": "`+topSum+`"},
	  "node_modules/vite/node_modules/lightningcss-linux-arm64-musl": {"os": ["linux"], "cpu": ["arm64"], "libc": ["musl"], "resolved": "`+url+`/nested.tgz", "integrity": "`+nestedSum+`"}
	}}`, "node_modules/vite")

	if err := fetchContainerBindings(context.Background(), dir, "arm64", io.Discard); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"node_modules/lightningcss-linux-arm64-musl":                   "1.32.0",
		"node_modules/vite/node_modules/lightningcss-linux-arm64-musl": "1.33.0",
	} {
		got, err := os.ReadFile(filepath.Join(dir, path, "package.json"))
		if err != nil || !strings.Contains(string(got), want) {
			t.Errorf("%s: got %q (%v), want version %s", path, got, err, want)
		}
	}
	if left := missingContainerBindings(dir, "arm64"); len(left) != 0 {
		t.Errorf("still missing after the fetch: %v", left)
	}
}

func TestFetchContainerBindings_RefusesAMismatchedTarball(t *testing.T) {
	data, _ := npmTarball(t, "x")
	_, otherSum := npmTarball(t, "y")
	url := registry(t, map[string][]byte{"/x.tgz": data})
	dir := t.TempDir()
	writeLock(t, dir, `{"packages": {"node_modules/x-linux-arm64-musl": {"os": ["linux"], "cpu": ["arm64"], "libc": ["musl"], "resolved": "`+url+`/x.tgz", "integrity": "`+otherSum+`"}}}`)

	err := fetchContainerBindings(context.Background(), dir, "arm64", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "x-linux-arm64-musl") {
		t.Fatalf("err = %v, want an integrity error naming the package", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "node_modules", "x-linux-arm64-musl")); statErr == nil {
		t.Error("a tarball that failed its check was installed")
	}
}

func TestFetchContainerBindings_NamesAnEntryWithoutASource(t *testing.T) {
	dir := t.TempDir()
	writeLock(t, dir, `{"packages": {"node_modules/x-linux-arm64-musl": {"os": ["linux"], "cpu": ["arm64"], "libc": ["musl"]}}}`)
	err := fetchContainerBindings(context.Background(), dir, "arm64", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "x-linux-arm64-musl") {
		t.Fatalf("err = %v, want one naming the package", err)
	}
}

func TestFetchContainerBindings_NothingMissingFetchesNothing(t *testing.T) {
	dir := t.TempDir()
	writeLock(t, dir, rollupLock, "node_modules/@rollup/rollup-linux-arm64-musl")
	var out strings.Builder
	if err := fetchContainerBindings(context.Background(), dir, "arm64", &out); err != nil || out.Len() != 0 {
		t.Fatalf("err = %v, output %q; want a silent no-op", err, out.String())
	}
}
