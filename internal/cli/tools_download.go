package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/download"
	"github.com/geodro/lerd/internal/tools"
)

// pinnedTools resolves host-tool download URLs for the current platform,
// loading the manifest lazily so an install with nothing missing never
// fetches it. Returns the pinned version so callers can stamp it.
type pinnedTools struct{ m *tools.Manifest }

func (p *pinnedTools) download(name, dest string, mode os.FileMode, w io.Writer) (string, error) {
	if p.m == nil {
		p.m = tools.Load(context.Background())
	}
	url, err := p.m.URL(name, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := download.File(context.Background(), url, dest, mode, w); err != nil {
		return "", err
	}
	return p.m.Tools[name].Version, nil
}

// ensureFnmBinary installs fnm into BinDir when it is missing. Called from
// downloadBinaries on a normal (fnm) install, and on demand when switching
// back to fnm with `lerd node:manager fnm` after an nvm-only setup.
func ensureFnmBinary(w io.Writer) error {
	if _, err := os.Stat(filepath.Join(config.BinDir(), "fnm")); err == nil {
		return nil
	}
	var pins pinnedTools
	return installFnm(&pins, w)
}

// installFnm downloads and extracts the pinned fnm, overwriting any existing
// binary, and stamps the installed version.
func installFnm(pins *pinnedTools, w io.Writer) error {
	binDir := config.BinDir()
	fnmZip := filepath.Join(binDir, "fnm.zip")
	v, err := pins.download("fnm", fnmZip, 0644, w)
	if err != nil {
		return fmt.Errorf("fnm download: %w", err)
	}
	extractCmd := exec.Command("unzip", "-o", fnmZip, "fnm", "-d", binDir)
	extractCmd.Stdout = w
	extractCmd.Stderr = w
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("fnm extract: %w", err)
	}
	os.Remove(fnmZip)
	os.Chmod(filepath.Join(binDir, "fnm"), 0755) //nolint:errcheck
	_ = tools.WriteStamp("fnm", v)
	return nil
}

// replaceTool downloads the pinned build of a single-binary tool next to its
// destination and swaps it in atomically, then stamps the version.
func replaceTool(pins *pinnedTools, name, dest string, w io.Writer) error {
	v, err := pins.download(name, dest+".new", 0755, w)
	if err != nil {
		os.Remove(dest + ".new")
		return err
	}
	if err := os.Rename(dest+".new", dest); err != nil {
		return err
	}
	_ = tools.WriteStamp(name, v)
	return nil
}
