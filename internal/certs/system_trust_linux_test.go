//go:build linux

package certs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustCAInSystemStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lerd-mkcert-rootCA.crt")

	origPath, origUpdate := systemStoreCAFile, updateSystemTrust
	t.Cleanup(func() { systemStoreCAFile, updateSystemTrust = origPath, origUpdate })
	systemStoreCAFile = path

	var updates int
	updateSystemTrust = func() error { updates++; return nil }

	ca := []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")
	if err := TrustCAInSystemStore(ca); err != nil {
		t.Fatalf("first install: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(ca) {
		t.Fatalf("CA not written: got %q err %v", got, err)
	}
	if updates != 1 {
		t.Errorf("update-ca-certificates called %d times, want 1", updates)
	}

	// Idempotent: same CA already in place must not rewrite or re-run update.
	if err := TrustCAInSystemStore(ca); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if updates != 1 {
		t.Errorf("idempotent install re-ran update (%d times)", updates)
	}
}
