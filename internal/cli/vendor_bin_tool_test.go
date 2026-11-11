package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/tools"
)

// frameworkWithTool installs a store-like framework naming tool for wp, and
// returns a project resolving to it. Pins come from the embedded manifest.
func frameworkWithTool(t *testing.T, tool string) string {
	t.Helper()
	isolateLerdHome(t)
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	fwDir := config.FrameworksDir()
	if err := os.MkdirAll(fwDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "name: wordpress\nversion: \"6\"\nlabel: WordPress\nvendor_bin_args:\n  wp:\n    - --allow-root\nvendor_bin_tools:\n  wp: " + tool + "\n"
	if err := os.WriteFile(filepath.Join(fwDir, "wordpress.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: wordpress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fakeDownloads replaces the network fetch, counting calls and recording the
// digest each one was asked to verify.
func fakeDownloads(t *testing.T, fail error) (*int, *string) {
	t.Helper()
	calls, digest := 0, ""
	orig := verifiedDownload
	verifiedDownload = func(_ context.Context, _, dest string, mode os.FileMode, sha string, _ io.Writer) error {
		calls++
		digest = sha
		if fail != nil {
			return fail
		}
		return os.WriteFile(dest, []byte("<?php echo 'wp';"), mode)
	}
	t.Cleanup(func() { verifiedDownload = orig })
	return &calls, &digest
}

func TestVendorBinTool_DownloadsOnceAndStamps(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	calls, digest := fakeDownloads(t, nil)

	path, ok, err := vendorBinTool(dir, "wp", io.Discard)
	if err != nil || !ok || path != tools.Path("wp-cli") {
		t.Fatalf("vendorBinTool = %q, %v, %v; want %s", path, ok, err, tools.Path("wp-cli"))
	}
	m := tools.Load(context.Background())
	if want := m.Digest("wp-cli", "linux", "amd64"); *digest != want {
		t.Errorf("download verified against %q, want the pinned %q", *digest, want)
	}
	if got := tools.InstalledVersion("wp-cli"); got != m.Tools["wp-cli"].Version {
		t.Errorf("installed version %q, want the pin stamped so the Tools page shows it", got)
	}
	if _, _, err := vendorBinTool(dir, "wp", io.Discard); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Errorf("downloaded %d times, want once", *calls)
	}
}

// The project's own wp-cli wins, also when lerd wp runs from a subdirectory.
func TestVendorBinTool_ProjectBinaryWins(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	calls, _ := fakeDownloads(t, nil)
	if err := os.MkdirAll(filepath.Join(dir, "vendor", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor", "bin", "wp"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "web", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{dir, sub} {
		if _, ok, err := vendorBinTool(cwd, "wp", io.Discard); ok || err != nil {
			t.Errorf("from %s: ok=%v err=%v, want the project's own binary to win", cwd, ok, err)
		}
	}
	if *calls != 0 {
		t.Errorf("downloaded %d times with vendor/bin/wp present", *calls)
	}
}

// A store definition naming a tool this binary does not know is refused with
// the key and the value, never guessed at.
func TestVendorBinTool_RefusesAnUnknownTool(t *testing.T) {
	for _, tool := range []string{"nope", "composer"} {
		dir := frameworkWithTool(t, tool)
		calls, _ := fakeDownloads(t, nil)
		_, _, err := vendorBinTool(dir, "wp", io.Discard)
		if err == nil || !strings.Contains(err.Error(), "vendor_bin_tools.wp") || !strings.Contains(err.Error(), tool) {
			t.Errorf("%s: err = %v, want a refusal naming vendor_bin_tools.wp and the tool", tool, err)
		}
		if *calls != 0 {
			t.Errorf("%s: a refused definition must not download anything", tool)
		}
	}
}

func TestVendorBinTool_UndeclaredOrNoFramework(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	fakeDownloads(t, nil)
	if _, ok, err := vendorBinTool(dir, "phpunit", io.Discard); ok || err != nil {
		t.Errorf("undeclared binary: ok=%v err=%v", ok, err)
	}
	isolateLerdHome(t)
	if _, ok, err := vendorBinTool(t.TempDir(), "wp", io.Discard); ok || err != nil {
		t.Errorf("no framework: ok=%v err=%v", ok, err)
	}
}

// A cloned repo must not be able to make lerd download and run a tool.
func TestVendorBinTool_IgnoresProjectDefinition(t *testing.T) {
	isolateLerdHome(t)
	t.Setenv("LERD_TOOLS_URL", "http://127.0.0.1:1/tools.yaml")
	calls, _ := fakeDownloads(t, nil)
	dir := t.TempDir()
	body := "framework: wordpress\nframework_def:\n  name: wordpress\n  version: \"6\"\n  vendor_bin_tools:\n    wp: wp-cli\n"
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := vendorBinTool(dir, "wp", io.Discard); ok || err != nil {
		t.Errorf("project-declared tool: ok=%v err=%v, want ignored", ok, err)
	}
	if *calls != 0 {
		t.Error("a project-declared tool must never be downloaded")
	}
}

// Offline or a bad mirror: say what failed and how else to get it, leave
// nothing behind, and try again next time.
func TestVendorBinTool_FailedDownload(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	calls, _ := fakeDownloads(t, errors.New("network is unreachable"))
	for range 2 {
		_, ok, err := vendorBinTool(dir, "wp", io.Discard)
		if ok || err == nil || !strings.Contains(err.Error(), "wp-cli") || !strings.Contains(err.Error(), "lerd composer require") {
			t.Fatalf("ok=%v err=%v, want an error naming wp-cli and the composer way", ok, err)
		}
	}
	if *calls != 2 {
		t.Errorf("download attempted %d times, want a retry on the next run", *calls)
	}
	if _, err := os.Stat(tools.Path("wp-cli")); err == nil {
		t.Error("a failed download left a tool behind")
	}
}

// With a tool declared, a missing vendor/bin/wp is not an error to report.
func TestMissingVendorBin_SilentWhenToolDeclared(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	if err := MissingVendorBin(dir, "wp"); err != nil {
		t.Errorf("MissingVendorBin = %v, want nil when a tool can stand in", err)
	}
}

// The Tools page's Update button reaches a fetched on-demand tool, and only
// once it has been fetched.
func TestUpdateOneTool_OnDemandOnceFetched(t *testing.T) {
	frameworkWithTool(t, "wp-cli")
	var updated []string
	orig := updateToolFn
	updateToolFn = func(_ *pinnedTools, name string) error { updated = append(updated, name); return nil }
	t.Cleanup(func() { updateToolFn = orig })

	if err := UpdateOneTool("wp-cli"); err == nil {
		t.Error("updating a tool nothing fetched yet should be refused")
	}
	if err := os.MkdirAll(filepath.Dir(tools.Path("wp-cli")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tools.Path("wp-cli"), []byte("phar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateOneTool("wp-cli"); err != nil || len(updated) != 1 {
		t.Errorf("UpdateOneTool = %v, updated %v; want wp-cli updated", err, updated)
	}
}

// Plain `wp` works once lerd has the tool: a shim in lerd's bin dir hands it
// to lerd wp. A wp the user installed, on PATH or in that dir, is left alone.
func TestVendorBinTool_WritesShim(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	fakeDownloads(t, nil)
	t.Setenv("PATH", t.TempDir())
	if _, _, err := vendorBinTool(dir, "wp", io.Discard); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(config.BinDir(), "wp"))
	if err != nil || !strings.Contains(string(body), `" wp "$@"`) {
		t.Fatalf("shim = %q, %v; want it to exec lerd wp", body, err)
	}
	if info, _ := os.Stat(filepath.Join(config.BinDir(), "wp")); info.Mode().Perm()&0o100 == 0 {
		t.Error("shim is not executable")
	}
}

func TestVendorBinTool_LeavesTheUsersWpAlone(t *testing.T) {
	dir := frameworkWithTool(t, "wp-cli")
	fakeDownloads(t, nil)
	own := t.TempDir()
	if err := os.WriteFile(filepath.Join(own, "wp"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", own)
	if _, _, err := vendorBinTool(dir, "wp", io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.BinDir(), "wp")); err == nil {
		t.Error("wrote a wp shim although the user has a wp of their own on PATH")
	}

	t.Setenv("PATH", t.TempDir())
	foreign := filepath.Join(config.BinDir(), "wp")
	if err := os.MkdirAll(config.BinDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := vendorBinTool(dir, "wp", io.Discard); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(foreign); string(body) != "#!/bin/sh\necho mine\n" {
		t.Errorf("overwrote a wp lerd did not write: %q", body)
	}
}

// A tool a newer published manifest adds is still unknown to this binary: its
// paths and the Tools page come from the pins it shipped with, so refuse it.
func TestVendorBinTool_RefusesAToolOnlyAPublishedManifestKnows(t *testing.T) {
	dir := frameworkWithTool(t, "newtool")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "tools:\n  newtool:\n    version: \"1.0.0\"\n    url: https://github.com/x/y/releases/download/{version}/newtool.phar\n    on_demand: true\n")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LERD_TOOLS_URL", srv.URL+"/tools.yaml")
	t.Setenv("LERD_TOOLS_HOSTS", "127.0.0.1")
	if _, ok := tools.Load(context.Background()).Tools["newtool"]; !ok {
		t.Fatal("test setup: the published manifest did not reach Load")
	}
	calls, _ := fakeDownloads(t, nil)
	_, _, err := vendorBinTool(dir, "wp", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "newtool") {
		t.Errorf("err = %v, want a refusal naming newtool", err)
	}
	if *calls != 0 {
		t.Error("a tool this binary does not ship must not be downloaded")
	}
}
