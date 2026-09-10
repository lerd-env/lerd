package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// writeVendorBin drops name into <dir>/vendor/bin with the given content and
// returns its path.
func writeVendorBin(t *testing.T, dir, name, content string) string {
	t.Helper()
	binDir := filepath.Join(dir, "vendor", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(binDir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVendorBinIsPHP(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		// wp-cli and drush ship a POSIX shell wrapper as their composer binary;
		// running those through `php` prints the script instead of executing it.
		{"wp", "#!/usr/bin/env sh\n\nexec \"${dir}/wp\" \"$@\"\n", false},
		{"drush", "#!/bin/sh\nexec php drush.php \"$@\"\n", false},
		{"bash-tool", "#!/usr/bin/env bash\necho hi\n", false},
		{"phpunit", "#!/usr/bin/env php\n<?php echo 1;\n", true},
		{"pest", "#!/usr/bin/php\n<?php echo 1;\n", true},
		{"noshebang", "<?php echo 1;\n", true},
		{"empty", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeVendorBin(t, dir, c.name, c.content)
			if got := vendorBinIsPHP(path); got != c.want {
				t.Errorf("vendorBinIsPHP(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// A path that isn't there at all keeps the php route, so the failure the user
// sees is php's own "could not open input file" rather than a silent no-op.
func TestVendorBinIsPHP_Missing(t *testing.T) {
	if !vendorBinIsPHP(filepath.Join(t.TempDir(), "nope")) {
		t.Error("missing binary should default to the php route")
	}
}

func TestVendorBinExecArgs_NonPHPRunsBinaryDirectly(t *testing.T) {
	dir := t.TempDir()
	args := vendorBinExecArgs(dir, "lerd-php84-fpm", "vendor/bin/wp", []string{"core", "version"}, false)

	// The binary itself is the exec entrypoint: no "php" ahead of it, or the
	// container prints the wrapper's source instead of running it.
	idx := -1
	for i, a := range args {
		if a == "lerd-php84-fpm" {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(args) {
		t.Fatalf("container name not followed by a command: %v", args)
	}
	if args[idx+1] != "vendor/bin/wp" {
		t.Errorf("entrypoint = %q, want vendor/bin/wp (args: %v)", args[idx+1], args)
	}
	if got := args[len(args)-2:]; got[0] != "core" || got[1] != "version" {
		t.Errorf("trailing args = %v, want [core version]", got)
	}
}

// The wrapper resolves `php` and its own siblings off PATH, so the exec has to
// carry the same PATH `lerd php` builds.
func TestVendorBinExecArgs_CarriesVendorBinOnPath(t *testing.T) {
	dir := t.TempDir()
	args := vendorBinExecArgs(dir, "lerd-php84-fpm", "vendor/bin/wp", nil, false)
	want := filepath.Join(dir, "vendor", "bin")
	found := false
	for i, a := range args {
		if a == "--env" && i+1 < len(args) && strings.HasPrefix(args[i+1], "PATH=") {
			if !strings.Contains(args[i+1], want) {
				t.Errorf("PATH %q missing %q", args[i+1], want)
			}
			found = true
		}
	}
	if !found {
		t.Error("no PATH env flag in exec args")
	}
}

// Under the native runtime the wrapper runs on the host, where it still has to
// find php (lerd's shim dir) and any sibling composer binary it shells out to.
func TestHostVendorBinPath_HasProjectBinAndShimDir(t *testing.T) {
	dir := t.TempDir()
	got := hostVendorBinPath(dir)
	if !strings.HasPrefix(got, filepath.Join(dir, "vendor", "bin")+string(os.PathListSeparator)) {
		t.Errorf("project vendor/bin must come first, got %q", got)
	}
	if !strings.Contains(got, config.BinDir()) {
		t.Errorf("PATH %q missing lerd's shim dir %q", got, config.BinDir())
	}
}

// A binary that isn't installed is named here rather than surfacing as an
// exec failure that blames the shell.
func TestRunHostVendorBin_MissingBinaryIsNamed(t *testing.T) {
	err := RunHostVendorBin(t.TempDir(), "vendor/bin/sail", nil)
	if err == nil {
		t.Fatal("want an error for a binary that is not installed")
	}
	if !strings.Contains(err.Error(), "vendor/bin/sail") {
		t.Errorf("error should name the binary, got %v", err)
	}
}
