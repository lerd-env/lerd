package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/tools"
)

const vendorBinShimMarker = "lerd vendor-bin tool shim"

// vendorBinTool returns the lerd tool standing in for vendor/bin/<name>,
// fetching it on first use; ok is false when the project has its own binary or
// no trusted definition names a tool (a project's framework_def cannot).
func vendorBinTool(cwd, name string, w io.Writer) (path string, ok bool, err error) {
	tool, declared := declaredVendorBinTool(cwd, name)
	if !declared {
		return "", false, nil
	}
	if !tools.OnDemand(tool) {
		return "", false, fmt.Errorf("vendor_bin_tools.%s: lerd has no on-demand tool %q; update lerd", name, tool)
	}
	pins := pinnedTools{m: tools.Load(context.Background())}
	path = tools.Path(tool)
	writeVendorBinShim(name)
	if _, err := os.Stat(path); err == nil {
		return path, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	if err := replaceTool(&pins, tool, path, w); err != nil {
		return "", false, fmt.Errorf("could not download %s for %s (%w); install it in the project with lerd composer require instead", tool, name, err)
	}
	return path, true, nil
}

// declaredVendorBinTool returns the tool the framework names for name when
// the project has no vendor/bin/<name> of its own. It fetches nothing.
func declaredVendorBinTool(cwd, name string) (string, bool) {
	root := projectRootFromCwd(cwd)
	if VendorBinExists(root, name) {
		return "", false
	}
	fw := trustedFrameworkForDir(root)
	if fw == nil {
		return "", false
	}
	tool, declared := fw.VendorBinTools[name]
	return tool, declared && tool != ""
}

// HasVendorBinTool reports whether a lerd tool stands in for lerd <name> here,
// so the dispatcher routes it to vendor-bin instead of cobra's unknown command.
func HasVendorBinTool(cwd, name string) bool {
	_, declared := declaredVendorBinTool(cwd, name)
	return declared
}

// writeVendorBinShim puts <name> on PATH as `lerd <name>`, so plain wp works
// in a site once lerd has a tool for it. Best effort: a <name> the user has,
// on PATH or already in lerd's bin dir, is theirs and is never replaced.
func writeVendorBinShim(name string) {
	binDir := config.BinDir()
	shim := filepath.Join(binDir, name)
	if body, err := os.ReadFile(shim); err == nil && !strings.Contains(string(body), vendorBinShimMarker) {
		return
	}
	if found, err := exec.LookPath(name); err == nil && filepath.Dir(found) != binDir {
		return
	}
	body := fmt.Sprintf("#!/bin/sh\n# %s\nexec %q %s \"$@\"\n", vendorBinShimMarker, config.LerdBinary(), name)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(shim, []byte(body), 0o755)
}
