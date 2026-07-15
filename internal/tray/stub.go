//go:build nogui

package tray

import (
	"os"
	"os/exec"
	"syscall"
)

// Run tries to exec the lerd-tray helper binary installed alongside lerd.
// If the helper is absent or its shared library is missing the error is
// silently swallowed — the rest of lerd keeps working without a tray.
func Run(mono bool) error {
	helper := HelperPath()
	if helper == "" {
		return nil
	}
	if _, err := os.Stat(helper); err != nil {
		return nil // helper not installed
	}

	var args []string
	if !mono {
		args = append(args, "--mono=false")
	}

	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil
	}
	defer null.Close()

	cmd := exec.Command(helper, args...)
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start() // ignore error — missing library, permissions, etc.
	return nil
}
