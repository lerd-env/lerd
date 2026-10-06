package cli

import (
	"errors"
	"path/filepath"
)

// checkUnattendedSupported refuses --unattended: the flag skips the sudo-gated
// steps because `lerd bootstrap` does them as root around it, and bootstrap is
// Linux-only. Here it would silently leave the resolver grant unwritten and the
// CA untrusted, which reads as broken HTTPS and a watcher asking for a password
// rather than as a missing feature.
func checkUnattendedSupported(unattended bool) error {
	if !unattended {
		return nil
	}
	return errors.New("--unattended is for package installs on Linux, where `lerd bootstrap` applies the root-level setup around it; run `lerd install` without it")
}

// bashRCPath is where lerd writes its PATH line: Terminal launches bash as a
// login shell, which reads .bash_profile rather than .bashrc.
func bashRCPath(home string) string { return filepath.Join(home, ".bash_profile") }
