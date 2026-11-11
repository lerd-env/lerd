package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeOverlay puts a user overlay for name in the sandboxed frameworks dir.
func writeOverlay(t *testing.T, name, body string) {
	t.Helper()
	if err := os.MkdirAll(FrameworksDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(FrameworksDir(), name+".yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// acmeStore installs a store definition with its own nginx block and required
// services, so a test can tell a replacement from an addition.
func acmeStore(t *testing.T) {
	t.Helper()
	setConfigDir(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(StoreFrameworksDir(), 0755); err != nil {
		t.Fatal(err)
	}
	store := "name: acme\nrequires: [redis]\nnginx:\n  snippet: |\n    location /static { try_files $uri =404; }\n  fastcgi_params:\n    ACME_BASE: \"{{root}}\"\n"
	if err := os.WriteFile(filepath.Join(StoreFrameworksDir(), "acme.yaml"), []byte(store), 0644); err != nil {
		t.Fatal(err)
	}
}

// The overlay is the user's own file, so its nginx block reaches the site, and
// replaces the base's whole rather than merging into it.
func TestGetFramework_OverlayNginxReplacesTheBase(t *testing.T) {
	acmeStore(t)
	writeOverlay(t, "acme", "name: acme\nnginx:\n  snippet: |\n    location /vm-overlay { return 204; }\n  fastcgi_params:\n    VM_OVERLAY: \"{{root}}\"\n")

	fw, ok := GetFramework("acme")
	if !ok {
		t.Fatal("acme did not resolve")
	}
	if fw.Nginx == nil {
		t.Fatal("the overlay's nginx block was dropped")
	}
	if fw.Nginx.Snippet != "location /vm-overlay { return 204; }\n" {
		t.Errorf("snippet = %q, want the overlay's", fw.Nginx.Snippet)
	}
	if want := map[string]string{"VM_OVERLAY": "{{root}}"}; !reflect.DeepEqual(fw.Nginx.FastCGIParams, want) {
		t.Errorf("fastcgi_params = %v, want only the overlay's %v", fw.Nginx.FastCGIParams, want)
	}
}

// requires is honoured from a user overlay, as the docs promise, and replaces
// the base's list rather than adding to it.
func TestGetFramework_OverlayRequiresReplacesTheBase(t *testing.T) {
	acmeStore(t)
	writeOverlay(t, "acme", "name: acme\nrequires: [opensearch]\n")

	fw, _ := GetFramework("acme")
	if !reflect.DeepEqual(fw.Requires, []string{"opensearch"}) {
		t.Errorf("requires = %v, want only the overlay's [opensearch]", fw.Requires)
	}
}

// An overlay that only adds workers, or lists no services, leaves the base's
// nginx block and required services alone. An empty list counts as unset, since
// saving the overlay would drop it anyway.
func TestGetFramework_OverlayWithoutNginxOrRequiresKeepsTheBase(t *testing.T) {
	acmeStore(t)
	writeOverlay(t, "acme", "name: acme\nrequires: []\nworkers:\n  pulse:\n    command: php acme pulse\n")

	fw, ok := GetFramework("acme")
	if !ok {
		t.Fatal("acme did not resolve")
	}
	if fw.Nginx == nil || fw.Nginx.Snippet != "location /static { try_files $uri =404; }\n" {
		t.Errorf("nginx = %+v, want the store definition's", fw.Nginx)
	}
	if !reflect.DeepEqual(fw.Requires, []string{"redis"}) {
		t.Errorf("requires = %v, want the store definition's [redis]", fw.Requires)
	}
	if fw.Workers["pulse"].Command != "php acme pulse" {
		t.Error("the overlay's worker was not merged")
	}
}

// lerd framework add saves a laravel overlay through SaveFramework, which must
// keep every key the merge honours or the overlay loses them before it is read.
func TestSaveFramework_LaravelOverlayKeepsNginxAndRequires(t *testing.T) {
	setConfigDir(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	in := &Framework{
		Name:     "laravel",
		Nginx:    &FrameworkNginx{Snippet: "location = /vm-overlay { return 204; }\n"},
		Requires: []string{"opensearch"},
	}
	if err := SaveFramework(in); err != nil {
		t.Fatal(err)
	}

	fw, _ := GetFramework("laravel")
	if fw.Nginx == nil || fw.Nginx.Snippet != in.Nginx.Snippet {
		t.Errorf("nginx = %+v, want the saved overlay's", fw.Nginx)
	}
	if !reflect.DeepEqual(fw.Requires, []string{"opensearch"}) {
		t.Errorf("requires = %v, want [opensearch]", fw.Requires)
	}
}
