package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLerdstead(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lerdstead.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadLerdstead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := writeLerdstead(t, `
sites:
  - path: ~/code/blog
    domains: [blog, admin.blog]
    php_version: "8.3"
    secured: true
    services: [mysql, redis]
  - path: /srv/shop
    secured: false
  - path: ~/code/plain

services: [mysql@8.4]
park: [~/clients]
`)

	ls, err := LoadLerdstead(path)
	if err != nil {
		t.Fatalf("LoadLerdstead: %v", err)
	}
	if len(ls.Sites) != 3 {
		t.Fatalf("sites = %d, want 3", len(ls.Sites))
	}

	blog := ls.Sites[0]
	if blog.Path != filepath.Join(home, "code", "blog") {
		t.Errorf("tilde not expanded: %q", blog.Path)
	}
	if len(blog.Domains) != 2 || blog.Domains[0] != "blog" {
		t.Errorf("domains = %v", blog.Domains)
	}
	if blog.PHPVersion != "8.3" {
		t.Errorf("php_version = %q", blog.PHPVersion)
	}
	if blog.Secured == nil || !*blog.Secured {
		t.Errorf("secured = %v, want true", blog.Secured)
	}
	if len(blog.Services) != 2 {
		t.Errorf("services = %v", blog.Services)
	}

	if shop := ls.Sites[1]; shop.Secured == nil || *shop.Secured {
		t.Errorf("explicit secured: false must parse as *false, got %v", shop.Secured)
	}
	if plain := ls.Sites[2]; plain.Secured != nil {
		t.Errorf("absent secured must stay nil, got %v", *plain.Secured)
	}

	if len(ls.Services) != 1 || ls.Services[0] != "mysql@8.4" {
		t.Errorf("global services = %v", ls.Services)
	}
	if len(ls.Park) != 1 || ls.Park[0] != filepath.Join(home, "clients") {
		t.Errorf("park = %v", ls.Park)
	}
}

func TestLoadLerdsteadRejectsMissingPath(t *testing.T) {
	path := writeLerdstead(t, "sites:\n  - domains: [blog]\n")
	if _, err := LoadLerdstead(path); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("want missing-path error, got %v", err)
	}
}

func TestLoadLerdsteadRejectsDuplicatePath(t *testing.T) {
	path := writeLerdstead(t, `
sites:
  - path: /srv/app
  - path: /srv/app
`)
	if _, err := LoadLerdstead(path); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("want duplicate-path error, got %v", err)
	}
}

func TestLoadLerdsteadRejectsUnknownField(t *testing.T) {
	path := writeLerdstead(t, `
sites:
  - path: /srv/app
    php_versoin: "8.3"
`)
	if _, err := LoadLerdstead(path); err == nil {
		t.Fatal("want error on unknown field (typo), got nil")
	}
}

func TestLoadLerdsteadMissingFile(t *testing.T) {
	_, err := LoadLerdstead(filepath.Join(t.TempDir(), "absent.yml"))
	if !os.IsNotExist(err) {
		t.Fatalf("want IsNotExist error, got %v", err)
	}
}

func TestSiteLerdsteadFlagRoundTrips(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if err := AddSite(Site{Name: "blog", Domains: []string{"blog.test"}, Path: "/srv/blog", Lerdstead: true}); err != nil {
		t.Fatalf("AddSite: %v", err)
	}
	site, err := FindSite("blog")
	if err != nil {
		t.Fatalf("FindSite: %v", err)
	}
	if !site.Lerdstead {
		t.Error("Lerdstead flag was not persisted through sites.yaml")
	}
}

func TestLerdsteadFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	want := filepath.Join(tmp, "lerd", "lerdstead.yml")
	if got := LerdsteadFile(); got != want {
		t.Errorf("LerdsteadFile() = %q, want %q", got, want)
	}
}
