package config

import (
	"os"
	"path/filepath"
	"testing"
)

// installFrameworkWithPublicDir writes a store definition serving from the
// given subdirectory, so a site can be resolved against a real declaration.
func installFrameworkWithPublicDir(t *testing.T, name, publicDir string) {
	t.Helper()
	dir := StoreFrameworksDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "name: " + name + "\nlabel: " + name + "\npublic_dir: " + publicDir + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
}

// serveFrom creates dir/sub with an index.php in it, the marker that says a
// directory is a real document root.
func serveFrom(t *testing.T, dir, sub string) {
	t.Helper()
	root := filepath.Join(dir, sub)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A public dir guessed at link time is frozen in the registry and never
// revisited. A Drupal project linked before composer install has an empty web/,
// so the guess lands on the project root, and every request after that answers
// 403 against a root with nothing to serve while the definition names the one
// that has it.
func TestPublicDirFor_AGuessThatCannotServeGivesWayToTheDefinition(t *testing.T) {
	setConfigDir(t)
	installFrameworkWithPublicDir(t, "drupalish", "web")

	dir := t.TempDir()
	serveFrom(t, dir, "web")

	site := Site{Name: "drupal", Path: dir, Framework: "drupalish", PublicDir: "."}
	if got := PublicDirFor(site); got != "web" {
		t.Errorf("PublicDirFor = %q, want web (the definition's root, which has an index.php)", got)
	}
}

// The definition only wins when it names a root that can actually serve. A
// project whose dependencies are not installed yet keeps what it has rather
// than being pointed at an empty directory.
func TestPublicDirFor_DefinitionRootWithoutAnIndexIsNotForced(t *testing.T) {
	setConfigDir(t)
	installFrameworkWithPublicDir(t, "drupalish", "web")

	dir := t.TempDir()

	site := Site{Name: "drupal", Path: dir, Framework: "drupalish", PublicDir: "."}
	if got := PublicDirFor(site); got != "." {
		t.Errorf("PublicDirFor = %q, want . (nothing better exists yet)", got)
	}
}

// A root the site records and can serve from is its own answer, so a project
// deliberately served from somewhere the definition does not name keeps it.
func TestPublicDirFor_RecordedRootThatServesIsKept(t *testing.T) {
	setConfigDir(t)
	installFrameworkWithPublicDir(t, "laravelish", "public")

	dir := t.TempDir()
	serveFrom(t, dir, "public_html")
	serveFrom(t, dir, "public")

	site := Site{Name: "app", Path: dir, Framework: "laravelish", PublicDir: "public_html"}
	if got := PublicDirFor(site); got != "public_html" {
		t.Errorf("PublicDirFor = %q, want public_html", got)
	}
}

// The project's own .lerd.yaml is the committed choice and outranks both, so a
// project that moves its document root is served from the new one without a
// relink.
func TestPublicDirFor_ProjectDeclarationWins(t *testing.T) {
	setConfigDir(t)
	installFrameworkWithPublicDir(t, "laravelish", "public")

	dir := t.TempDir()
	serveFrom(t, dir, "public")
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("public_dir: httpdocs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	site := Site{Name: "app", Path: dir, Framework: "laravelish", PublicDir: "public"}
	if got := PublicDirFor(site); got != "httpdocs" {
		t.Errorf("PublicDirFor = %q, want httpdocs (declared in .lerd.yaml)", got)
	}
}

// A site whose registry entry names no framework still resolves through the one
// its project declares, so a stale registry cannot hide the definition.
func TestPublicDirFor_FrameworkDetectedWhenTheRegistryNamesNone(t *testing.T) {
	setConfigDir(t)
	installFrameworkWithPublicDir(t, "drupalish", "web")

	dir := t.TempDir()
	serveFrom(t, dir, "web")
	if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: drupalish\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	site := Site{Name: "drupal", Path: dir, PublicDir: "."}
	if got := PublicDirFor(site); got != "web" {
		t.Errorf("PublicDirFor = %q, want web", got)
	}
}

// Nothing anywhere leaves the long-standing default.
func TestPublicDirFor_DefaultsToPublic(t *testing.T) {
	setConfigDir(t)
	if got := PublicDirFor(Site{Name: "app", Path: t.TempDir()}); got != "public" {
		t.Errorf("PublicDirFor = %q, want public", got)
	}
}

// A root that would pivot the document root out of the project is refused
// wherever it comes from.
func TestPublicDirFor_RejectsAnEscapingRoot(t *testing.T) {
	setConfigDir(t)
	installFrameworkWithPublicDir(t, "laravelish", "public")

	dir := t.TempDir()
	serveFrom(t, dir, "public")

	site := Site{Name: "app", Path: dir, Framework: "laravelish", PublicDir: "../../etc"}
	if got := PublicDirFor(site); got != "public" {
		t.Errorf("PublicDirFor = %q, want public (the escaping root refused)", got)
	}
}
