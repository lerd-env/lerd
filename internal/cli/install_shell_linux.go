package cli

import "path/filepath"

// checkUnattendedSupported accepts --unattended: `lerd bootstrap --system` and
// `--trust-ca` apply the sudo-gated steps it skips as root around it.
func checkUnattendedSupported(bool) error { return nil }

// bashRCPath is where lerd writes its PATH line: interactive bash reads .bashrc.
func bashRCPath(home string) string { return filepath.Join(home, ".bashrc") }
