package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

// The URL key and the file holding it are declared by the framework, so a site
// served over https must have its own key rewritten, not a hardcoded APP_URL in
// a .env the framework does not even use.
func TestSyncPrimaryDomainWritesTheDeclaredKey(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(local, []byte("DEFAULT_URI=http://shop.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SyncPrimaryDomain(dir, ".env.local", "DEFAULT_URI", "shop.test", true); err != nil {
		t.Fatal(err)
	}
	if got := ReadKey(local, "DEFAULT_URI"); got != "https://shop.test" {
		t.Errorf("DEFAULT_URI = %q, want https://shop.test", got)
	}

	if err := SyncPrimaryDomain(dir, ".env.local", "DEFAULT_URI", "shop.test", false); err != nil {
		t.Fatal(err)
	}
	if got := ReadKey(local, "DEFAULT_URI"); got != "http://shop.test" {
		t.Errorf("DEFAULT_URI = %q, want http://shop.test", got)
	}
}

// url_key: none means the framework keeps its base URL elsewhere (Magento uses
// the database), so writing one into the env file would just be litter.
func TestSyncPrimaryDomainSkipsWhenNoURLKey(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_URL=http://x.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SyncPrimaryDomain(dir, ".env", "", "x.test", true); err != nil {
		t.Fatal(err)
	}
	if got := ReadKey(env, "APP_URL"); got != "http://x.test" {
		t.Errorf("APP_URL was rewritten for a framework that declares no URL key: %q", got)
	}
}

func TestUpdateAppURLWritesTheDeclaredKey(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(local, []byte("DEFAULT_URI=http://a.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateAppURL(dir, ".env.local", "DEFAULT_URI", "https", "b.test"); err != nil {
		t.Fatal(err)
	}
	if got := ReadKey(local, "DEFAULT_URI"); got != "https://b.test" {
		t.Errorf("DEFAULT_URI = %q, want https://b.test", got)
	}
}
