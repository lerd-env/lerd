//go:build linux

package certs

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
)

// systemStoreCAFile is where the mkcert root CA is dropped for the system trust
// store; update-ca-certificates picks up .crt files under this directory. A var
// so tests can point it at a temp path.
var systemStoreCAFile = "/usr/local/share/ca-certificates/lerd-mkcert-rootCA.crt"

// updateSystemTrust refreshes the aggregated trust bundle. A var so tests can
// stub out the real command.
var updateSystemTrust = func() error {
	return exec.Command("update-ca-certificates").Run()
}

// TrustCAInSystemStore installs the given mkcert root CA PEM into the system
// trust store. The caller must be root. Idempotent: a byte-identical CA already
// in place is a no-op, so re-running on every package upgrade is cheap.
//
// mkcert normally does this itself via an interactive sudo, which a package
// maintainer script cannot answer. `lerd bootstrap --trust-ca` runs as root and
// installs the user-generated CA directly instead.
func TrustCAInSystemStore(caPEM []byte) error {
	if existing, err := os.ReadFile(systemStoreCAFile); err == nil && bytes.Equal(existing, caPEM) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(systemStoreCAFile), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(systemStoreCAFile, caPEM, 0644); err != nil {
		return err
	}
	return updateSystemTrust()
}
