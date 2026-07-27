//go:build linux

package certs

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrNoSystemTrustStore reports that no known CA anchor directory exists on
// this system, as on NixOS and other distros where trust is declarative.
var ErrNoSystemTrustStore = errors.New("no system CA trust store found")

// systemTrustStore is one distro family's CA anchor directory and the command
// that rebuilds the aggregated trust bundle from it.
type systemTrustStore struct {
	dir      string
	filename string
	command  []string
}

// systemTrustStores lists the anchor layouts mkcert recognises, in the same
// order; the first whose directory exists wins. A var so tests can substitute
// temp paths.
var systemTrustStores = []systemTrustStore{
	{"/etc/pki/ca-trust/source/anchors", "lerd-mkcert-rootCA.pem", []string{"update-ca-trust", "extract"}},       // Fedora/RHEL
	{"/usr/local/share/ca-certificates", "lerd-mkcert-rootCA.crt", []string{"update-ca-certificates"}},           // Debian/Ubuntu
	{"/etc/ca-certificates/trust-source/anchors", "lerd-mkcert-rootCA.crt", []string{"trust", "extract-compat"}}, // Arch
	{"/usr/share/pki/trust/anchors", "lerd-mkcert-rootCA.crt", []string{"update-ca-certificates"}},               // openSUSE
}

// updateSystemTrust refreshes the aggregated trust bundle. A var so tests can
// stub out the real command.
var updateSystemTrust = func(command []string) error {
	return exec.Command(command[0], command[1:]...).Run()
}

// TrustCAInSystemStore installs the given mkcert root CA PEM into whichever
// system trust store this distro uses. The caller must be root. Idempotent: a
// byte-identical CA already in place is a no-op, so re-running on every package
// upgrade is cheap.
//
// mkcert normally does this itself via an interactive sudo, which a package
// maintainer script cannot answer. `lerd bootstrap --trust-ca` runs as root and
// installs the user-generated CA directly instead.
func TrustCAInSystemStore(caPEM []byte) error {
	for _, store := range systemTrustStores {
		if info, err := os.Stat(store.dir); err != nil || !info.IsDir() {
			continue
		}
		caFile := filepath.Join(store.dir, store.filename)
		if existing, err := os.ReadFile(caFile); err == nil && bytes.Equal(existing, caPEM) {
			return nil
		}
		if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
			return err
		}
		return updateSystemTrust(store.command)
	}
	return ErrNoSystemTrustStore
}

// UntrustCAFromSystemStore removes the anchor TrustCAInSystemStore wrote and
// refreshes the bundle. The caller must be root. mkcert's own -uninstall keys
// on the filename mkcert chose, so it never reaches this one; without this the
// CA would stay trusted on the machine after lerd is gone. Idempotent, and
// every store is swept rather than only the first so nothing is left behind.
func UntrustCAFromSystemStore() error {
	var sawStore bool
	for _, store := range systemTrustStores {
		if info, err := os.Stat(store.dir); err != nil || !info.IsDir() {
			continue
		}
		sawStore = true
		caFile := filepath.Join(store.dir, store.filename)
		if _, err := os.Stat(caFile); err != nil {
			continue
		}
		if err := os.Remove(caFile); err != nil {
			return err
		}
		if err := updateSystemTrust(store.command); err != nil {
			return err
		}
	}
	if !sawStore {
		return ErrNoSystemTrustStore
	}
	return nil
}

// SystemTrustAnchorPresent reports whether a lerd-written anchor is on disk, so
// an uninstall only escalates to root when there is something to remove.
func SystemTrustAnchorPresent() bool {
	for _, store := range systemTrustStores {
		if _, err := os.Stat(filepath.Join(store.dir, store.filename)); err == nil {
			return true
		}
	}
	return false
}
