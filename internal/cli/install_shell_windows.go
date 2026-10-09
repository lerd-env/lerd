package cli

import (
	"errors"
	"path/filepath"
)

// checkUnattendedSupported refuses --unattended: it relies on `lerd bootstrap`
// doing the root-level setup around it, and bootstrap is Linux-only.
func checkUnattendedSupported(unattended bool) error {
	if !unattended {
		return nil
	}
	return errors.New("--unattended is for package installs on Linux, where `lerd bootstrap` applies the root-level setup around it; run `lerd install` without it")
}

// bashRCPath is where lerd writes its PATH line for Git Bash, which reads .bashrc.
func bashRCPath(home string) string { return filepath.Join(home, ".bashrc") }
