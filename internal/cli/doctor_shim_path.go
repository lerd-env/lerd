package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
)

// shimShadowFinding compares the tool a shell resolves against the shim lerd
// installed. A host binary in front of the shim runs outside the container,
// where the service hostnames in a site's .env do not exist, so every CLI
// command fails on a name lookup while the browser keeps working.
func shimShadowFinding(tool, shim, resolved string, lookErr error) (status, detail string) {
	switch {
	case lookErr != nil:
		return "warn", "not on your PATH — lerd's shims dir is missing from your shell, run: lerd path:enable"
	case resolved == shim:
		return "ok", ""
	default:
		return "warn", fmt.Sprintf("%s leads instead of lerd's shim, so %s %s — move lerd's PATH line to the end of your shell rc", resolved, tool, shimShadowSymptom(tool))
	}
}

// shimShadowSymptom is what actually breaks when a host binary wins over the
// shim, which differs by tool: php leaves the container network, node leaves
// the version lerd pins.
func shimShadowSymptom(tool string) string {
	switch tool {
	case "node", "npm", "npx":
		return "runs under whatever Node your version manager has active rather than the Node default lerd pins, and a build can fail on a version the project never targeted"
	default:
		return "runs on the host and cannot reach the lerd-… hostnames in a site's .env"
	}
}

// resolvedShimPath returns the shim lerd installed for tool and what the
// current shell actually resolves, both with symlinks collapsed so a linked
// home or bin dir does not read as a shadow.
func resolvedShimPath(tool string) (shim, resolved string, err error) {
	shim = realPath(filepath.Join(config.BinDir(), tool))
	found, err := exec.LookPath(tool)
	if err != nil {
		return shim, "", err
	}
	return shim, realPath(found), nil
}

func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// shimInstalled reports whether lerd wrote a shim for tool at all; there is
// nothing to compare against on an install that never got one.
func shimInstalled(tool string) bool {
	_, err := os.Stat(filepath.Join(config.BinDir(), tool))
	return err == nil
}
