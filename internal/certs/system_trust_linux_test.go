//go:build linux

package certs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func stubTrustStores(t *testing.T, stores []systemTrustStore) *[][]string {
	t.Helper()
	origStores, origUpdate := systemTrustStores, updateSystemTrust
	t.Cleanup(func() { systemTrustStores, updateSystemTrust = origStores, origUpdate })
	systemTrustStores = stores

	var commands [][]string
	updateSystemTrust = func(command []string) error {
		commands = append(commands, command)
		return nil
	}
	return &commands
}

func TestTrustCAInSystemStore(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "anchors")
	if err := os.MkdirAll(present, 0755); err != nil {
		t.Fatal(err)
	}
	// The first candidate's directory does not exist, so detection must fall
	// through to the second.
	commands := stubTrustStores(t, []systemTrustStore{
		{filepath.Join(dir, "missing"), "lerd-mkcert-rootCA.pem", []string{"update-ca-trust", "extract"}},
		{present, "lerd-mkcert-rootCA.crt", []string{"update-ca-certificates"}},
	})

	ca := []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")
	if err := TrustCAInSystemStore(ca); err != nil {
		t.Fatalf("first install: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(present, "lerd-mkcert-rootCA.crt"))
	if err != nil || string(got) != string(ca) {
		t.Fatalf("CA not written: got %q err %v", got, err)
	}
	if len(*commands) != 1 || (*commands)[0][0] != "update-ca-certificates" {
		t.Errorf("trust command runs = %v, want one update-ca-certificates", *commands)
	}

	// Idempotent: same CA already in place must not rewrite or re-run update.
	if err := TrustCAInSystemStore(ca); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if len(*commands) != 1 {
		t.Errorf("idempotent install re-ran the trust command (%d runs)", len(*commands))
	}
}

func TestUntrustCAFromSystemStore(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "anchors")
	if err := os.MkdirAll(present, 0755); err != nil {
		t.Fatal(err)
	}
	commands := stubTrustStores(t, []systemTrustStore{
		{filepath.Join(dir, "missing"), "lerd-mkcert-rootCA.pem", []string{"update-ca-trust", "extract"}},
		{present, "lerd-mkcert-rootCA.crt", []string{"update-ca-certificates"}},
	})

	ca := []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")
	if err := TrustCAInSystemStore(ca); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !SystemTrustAnchorPresent() {
		t.Fatal("anchor reported absent right after install")
	}

	if err := UntrustCAFromSystemStore(); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(present, "lerd-mkcert-rootCA.crt")); !os.IsNotExist(err) {
		t.Errorf("anchor still on disk after removal (stat err %v)", err)
	}
	if SystemTrustAnchorPresent() {
		t.Error("anchor still reported present after removal")
	}
	if len(*commands) != 2 {
		t.Errorf("bundle refreshes = %d, want 2 (one per install/uninstall)", len(*commands))
	}

	// Idempotent: nothing left to remove means no second bundle refresh.
	if err := UntrustCAFromSystemStore(); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
	if len(*commands) != 2 {
		t.Errorf("uninstall with no anchor re-ran the trust command (%d runs)", len(*commands))
	}
}

func TestUntrustCAFromSystemStoreNoStore(t *testing.T) {
	dir := t.TempDir()
	stubTrustStores(t, []systemTrustStore{
		{filepath.Join(dir, "missing-a"), "a.pem", []string{"update-ca-trust", "extract"}},
	})
	if err := UntrustCAFromSystemStore(); !errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("err = %v, want ErrNoSystemTrustStore", err)
	}
}

func TestTrustCAInSystemStoreNoStore(t *testing.T) {
	dir := t.TempDir()
	commands := stubTrustStores(t, []systemTrustStore{
		{filepath.Join(dir, "missing-a"), "a.pem", []string{"update-ca-trust", "extract"}},
		{filepath.Join(dir, "missing-b"), "b.crt", []string{"update-ca-certificates"}},
	})

	err := TrustCAInSystemStore([]byte("ca"))
	if !errors.Is(err, ErrNoSystemTrustStore) {
		t.Fatalf("err = %v, want ErrNoSystemTrustStore", err)
	}
	if len(*commands) != 0 {
		t.Errorf("trust command ran with no store present: %v", *commands)
	}
}
