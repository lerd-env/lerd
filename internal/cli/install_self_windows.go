//go:build windows

package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// installSelf copies a lerd.exe run from anywhere else, such as a downloads
// folder, into lerd's bin dir and runs the install again from there, so the
// services, the shims and the PATH entry all name a copy that stays put.
func installSelf() (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, nil
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	binDir := config.BinDir()
	if sameExecutable(exe, filepath.Join(binDir, "lerd.exe")) {
		return false, nil
	}
	feedback.Line(fmt.Sprintf("Copying lerd to %s…", binDir))
	dest, err := placeBinary(exe, binDir)
	if err != nil {
		return true, fmt.Errorf("copying lerd into %s: %w", binDir, err)
	}
	cmd := exec.Command(dest, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	return true, err
}

// placeBinary copies lerd.exe, and lerd-tray.exe when it sits beside it, into
// binDir, moving aside a copy that is running there.
func placeBinary(exe, binDir string) (string, error) {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	for _, name := range []string{"lerd.exe", "lerd-tray.exe"} {
		src := filepath.Join(filepath.Dir(exe), name)
		if name == "lerd.exe" {
			src = exe
		} else if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := swapBinary(src, filepath.Join(binDir, name)); err != nil {
			return "", err
		}
	}
	return filepath.Join(binDir, "lerd.exe"), nil
}

// sameExecutable compares Windows paths, which ignore case and accept either
// separator.
func sameExecutable(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
