package siteinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// installWordPressDef writes a store definition shaped like the published
// wordpress one, so the enriched site has something to label itself with.
func installWordPressDef(t *testing.T) {
	t.Helper()
	dir := config.StoreFrameworksDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "name: wordpress\nlabel: WordPress\nversion: \"6\"\npublic_dir: .\ndetect:\n  - file: wp-login.php\n"
	if err := os.WriteFile(filepath.Join(dir, "wordpress@6.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A site registered while its definition could not resolve carries no framework
// in the registry, and nothing ever revisits that entry. The project's own
// .lerd.yaml is the committed truth, so the site has to read as what it declares
// rather than as nothing at all.
func TestEnrich_FrameworkFromProjectWhenRegistryIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	installWordPressDef(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: wordpress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := Enrich(config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: dir}, EnrichFramework)

	if e.FrameworkName != "wordpress" {
		t.Errorf("FrameworkName = %q, want wordpress", e.FrameworkName)
	}
	if e.FrameworkLabel != "WordPress 6" {
		t.Errorf("FrameworkLabel = %q, want WordPress 6", e.FrameworkLabel)
	}
}

// Detection from the project's files carries the same weight: a project that
// never committed a .lerd.yaml is still recognisable from what is in it.
func TestEnrich_FrameworkDetectedFromFilesWhenRegistryIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	installWordPressDef(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wp-login.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := Enrich(config.Site{Name: "blog", Domains: []string{"blog.test"}, Path: dir}, EnrichFramework)

	if e.FrameworkName != "wordpress" {
		t.Errorf("FrameworkName = %q, want wordpress detected from the project files", e.FrameworkName)
	}
}

// The registry still wins when it holds a name: a site deliberately registered
// as one framework is not re-read as another on every refresh.
func TestEnrich_RegistryFrameworkWins(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	installWordPressDef(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wp-login.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := Enrich(config.Site{Name: "app", Path: dir, Framework: "laravel"}, EnrichFramework)

	if e.FrameworkName != "laravel" {
		t.Errorf("FrameworkName = %q, want the registered laravel", e.FrameworkName)
	}
}

// A site with no framework anywhere stays without one, so a static project is
// not labelled by a stray file.
func TestEnrich_NoFrameworkAnywhere(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	e := Enrich(config.Site{Name: "static", Path: t.TempDir()}, EnrichFramework)

	if e.FrameworkName != "" || e.FrameworkLabel != "" {
		t.Errorf("FrameworkName/Label = %q/%q, want both empty", e.FrameworkName, e.FrameworkLabel)
	}
}
