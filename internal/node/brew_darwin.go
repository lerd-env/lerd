package node

import (
	"path/filepath"

	"github.com/geodro/lerd/internal/hostbin"
)

// homebrewBins are Homebrew's two prefixes, Apple Silicon then Intel, which
// launchd leaves off the PATH it hands lerd-ui and lerd-watcher.
var homebrewBins = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// misePrefixes are the package-manager dirs a daemon's restricted PATH misses.
func misePrefixes() []string { return homebrewBins }

// brewBunDirs are where Homebrew installs bun.
func brewBunDirs() []string { return homebrewBins }

// osUnitPathDirs puts the dirs a daemon misses on the worker unit's PATH.
func osUnitPathDirs() []string { return hostbin.ExtraDirs() }

// osBrewNvmScripts is <prefix>/opt/nvm/nvm.sh for every prefix hostbin knows.
func osBrewNvmScripts() []string {
	var out []string
	for _, binDir := range hostbin.ExtraDirs() {
		out = append(out, filepath.Join(filepath.Dir(binDir), "opt", "nvm", "nvm.sh"))
	}
	return out
}
