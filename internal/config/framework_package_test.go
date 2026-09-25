package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// packageSandbox lays out a store holding one acme@11 definition and an index
// that publishes the package layer, plus a project requiring the framework and
// whatever extra composer packages the caller names.
// packages are index entries in their JSON form, so a test can publish a package
// with versions of its own as easily as one without.
func packageSandbox(t *testing.T, packages []string, require ...string) (store, project string) {
	t.Helper()
	store = storeSandbox(t)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(store, "acme@11.yaml"), `name: acme
label: Acme
version: "11"
public_dir: public
detect:
  - composer: acme/framework
workers:
  scheduler:
    label: Scheduler
    command: php acme schedule:work
commands:
  - name: migrate
    label: Run migrations
    command: php acme migrate
`)

	index := `{"frameworks":[{"name":"acme","label":"Acme","versions":["11"],"latest":"11","detect":[{"composer":"acme/framework"}]}]`
	if len(packages) > 0 {
		index += `,"packages":[` + strings.Join(packages, ",") + `]`
	}
	write(filepath.Join(store, "index.json"), index+"}")

	project = t.TempDir()
	deps := `"acme/framework": "^11.0"`
	for _, r := range require {
		deps += `, "` + r + `": "^1.0"`
	}
	write(filepath.Join(project, "composer.json"), `{"require": {`+deps+`}}`)
	return store, project
}

// packageAtVersion is a one-worker definition pinned to a major of the package.
func packageAtVersion(version, command string) string {
	head := "package: acme/electron\n"
	if version != "" {
		head += "version: \"" + version + "\"\n"
	}
	return head + "workers:\n  native:\n    command: " + command + "\n"
}

const electronPackage = `package: acme/electron
frameworks:
  - name: acme
    min: "11"
workers:
  native:
    label: Native
    command: php acme native:serve
commands:
  - name: native:build
    label: Build desktop app
    command: php acme native:build
doctor:
  checks:
    - name: native_runtime
      type: command
      command: test -x vendor/acme/electron/php
`

// writePackage seeds the package cache, which is a sibling of the framework
// store rather than a directory inside it.
func writePackage(t *testing.T, slug, body string) {
	t.Helper()
	if err := os.MkdirAll(StorePackagesDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(StorePackagesDir(), slug+".yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetFrameworkForDir_packageDeclarationsMerge(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", electronPackage)

	fw, ok := GetFrameworkForDir("acme", project)
	if !ok {
		t.Fatal("framework did not resolve")
	}
	if _, has := fw.Workers["native"]; !has {
		t.Error("package worker was not merged")
	}
	if !hasCommandNamed(fw.Commands, "native:build") {
		t.Error("package command was not merged")
	}
	if fw.Doctor == nil || !hasCheckNamed(fw.Doctor.Checks, "native_runtime") {
		t.Error("package doctor check was not merged")
	}
	if _, has := fw.Workers["scheduler"]; !has {
		t.Error("framework's own worker was lost")
	}
}

func TestGetFrameworkForDir_packageSkippedWithoutTheComposerPackage(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`})
	writePackage(t, "acme-electron", electronPackage)

	fw, _ := GetFrameworkForDir("acme", project)
	if _, has := fw.Workers["native"]; has {
		t.Error("a project without the package must not get its worker")
	}
}

// A package the index does not publish is never read, so a file left behind by
// an earlier catalogue cannot keep injecting a worker.
func TestGetFrameworkForDir_packageOutsideTheIndexIsIgnored(t *testing.T) {
	_, project := packageSandbox(t, nil, "acme/electron")
	writePackage(t, "acme-electron", electronPackage)

	fw, _ := GetFrameworkForDir("acme", project)
	if _, has := fw.Workers["native"]; has {
		t.Error("package outside the index must not be merged")
	}
}

func TestGetFrameworkForDir_packageRangeExcludesOlderMajors(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", `package: acme/electron
frameworks:
  - name: acme
    min: "12"
workers:
  native:
    command: php acme native:serve
`)

	fw, _ := GetFrameworkForDir("acme", project)
	if _, has := fw.Workers["native"]; has {
		t.Error("acme 11 is below the package's range and must not get the worker")
	}
}

func TestGetFrameworkForDir_packageNarrowedToAnotherFramework(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", `package: acme/electron
frameworks:
  - name: other
workers:
  native:
    command: php acme native:serve
`)

	fw, _ := GetFrameworkForDir("acme", project)
	if _, has := fw.Workers["native"]; has {
		t.Error("a package scoped to another framework must not be merged")
	}
}

// The package is where the entry is maintained now, so a copy left behind in a
// version file is replaced rather than shadowing it.
func TestGetFrameworkForDir_packageWinsNameCollisions(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", `package: acme/electron
workers:
  scheduler:
    command: php acme package-scheduler
commands:
  - name: migrate
    label: Package migrations
    command: php acme package-migrate
`)

	fw, _ := GetFrameworkForDir("acme", project)
	if got := fw.Workers["scheduler"].Command; got != "php acme package-scheduler" {
		t.Errorf("worker command = %q, want the package's", got)
	}
	migrations := 0
	for _, c := range fw.Commands {
		if c.Name != "migrate" {
			continue
		}
		migrations++
		if c.Command != "php acme package-migrate" {
			t.Errorf("command = %q, want the package's", c.Command)
		}
	}
	if migrations != 1 {
		t.Errorf("migrate appears %d times, want the package's entry in place of the framework's", migrations)
	}
}

// The overlay is the user's own file, so it stays above the package layer the
// way it is above every store definition.
func TestGetFrameworkForDir_userOverlayWinsOverAPackage(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", electronPackage)
	overlay := filepath.Join(FrameworksDir(), "acme.yaml")
	if err := os.MkdirAll(filepath.Dir(overlay), 0755); err != nil {
		t.Fatal(err)
	}
	body := "name: acme\nworkers:\n  native:\n    command: php acme mine\n"
	if err := os.WriteFile(overlay, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	fw, _ := GetFrameworkForDir("acme", project)
	if got := fw.Workers["native"].Command; got != "php acme mine" {
		t.Errorf("worker command = %q, want the overlay's", got)
	}
}

func hasCommandNamed(cmds []FrameworkCommand, name string) bool {
	for _, c := range cmds {
		if c.Name == name {
			return true
		}
	}
	return false
}

func hasCheckNamed(checks []DoctorCheck, name string) bool {
	for _, c := range checks {
		if c.Name == name {
			return true
		}
	}
	return false
}

// Resolving twice must not accumulate: the parsed definition is cached and
// shared, so a merge that appended into it would grow the list every call.
func TestGetFrameworkForDir_packageMergeDoesNotAccumulate(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", electronPackage)

	first, _ := GetFrameworkForDir("acme", project)
	second, _ := GetFrameworkForDir("acme", project)
	if len(first.Commands) != len(second.Commands) {
		t.Errorf("commands grew between resolutions: %d then %d", len(first.Commands), len(second.Commands))
	}
	if len(first.Doctor.Checks) != len(second.Doctor.Checks) {
		t.Errorf("doctor checks grew between resolutions: %d then %d", len(first.Doctor.Checks), len(second.Doctor.Checks))
	}
}

func TestStorePackageFile_rejectsNamesThatEscapeTheStore(t *testing.T) {
	storeSandbox(t)
	for _, name := range []string{"../../etc/passwd", "acme/../../x", "acme", "acme/sub/dir", "/abs/path", "Acme/Electron", ""} {
		if got := StorePackageFile(name, ""); got != "" {
			t.Errorf("%q must be refused, got %q", name, got)
		}
	}
	if got := StorePackageFile("acme/electron", ""); filepath.Base(got) != "acme-electron.yaml" {
		t.Errorf("unversioned path = %q, want the slug file", got)
	}
	if got := StorePackageFile("acme/electron", "5"); filepath.Base(got) != "acme-electron@5.yaml" {
		t.Errorf("versioned path = %q, want the slug@major file", got)
	}
}

// A package whose declarations moved in one of its own majors publishes a file
// per major, and a project is served the newest at or below what it installed.
func TestGetFrameworkForDir_packageVersionFollowsTheInstalledPackage(t *testing.T) {
	cases := []struct {
		installed string
		want      string
	}{
		{"^5.0", "php acme five"},
		{"^6.2", "php acme six"},
		{"^9.0", "php acme six"},
		{"^3.0", "php acme base"},
	}
	for _, tc := range cases {
		t.Run(tc.installed, func(t *testing.T) {
			_, project := packageSandbox(t, []string{`{"name":"acme/electron","versions":["5","6"],"latest":"6"}`})
			if err := os.WriteFile(filepath.Join(project, "composer.json"),
				[]byte(`{"require": {"acme/framework": "^11.0", "acme/electron": "`+tc.installed+`"}}`), 0644); err != nil {
				t.Fatal(err)
			}
			writePackage(t, "acme-electron", packageAtVersion("", "php acme base"))
			writePackage(t, "acme-electron@5", packageAtVersion("5", "php acme five"))
			writePackage(t, "acme-electron@6", packageAtVersion("6", "php acme six"))

			fw, _ := GetFrameworkForDir("acme", project)
			if got := fw.Workers["native"].Command; got != tc.want {
				t.Errorf("worker command = %q, want %q", got, tc.want)
			}
		})
	}
}

// Below the first major that needed a file of its own, the unversioned file is
// what serves the project, so a package that breaks something adds one file
// instead of restating everything it did before.
func TestGetFrameworkForDir_packageBelowEveryVersionUsesTheBaseFile(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron","versions":["6"],"latest":"6"}`})
	if err := os.WriteFile(filepath.Join(project, "composer.json"),
		[]byte(`{"require": {"acme/framework": "^11.0", "acme/electron": "^4.0"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	writePackage(t, "acme-electron", packageAtVersion("", "php acme base"))
	writePackage(t, "acme-electron@6", packageAtVersion("6", "php acme six"))

	fw, _ := GetFrameworkForDir("acme", project)
	if got := fw.Workers["native"].Command; got != "php acme base" {
		t.Errorf("worker command = %q, want the unversioned file's", got)
	}
}

// The day the store starts publishing a major of its own, an install that cannot
// reach it is asked for a file it has never fetched. The newest cached file at
// or below that version answers instead of nothing.
func TestGetFrameworkForDir_packageFallsBackWhenTheWantedVersionIsNotCached(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron","versions":["5","7"],"latest":"7"}`})
	if err := os.WriteFile(filepath.Join(project, "composer.json"),
		[]byte(`{"require": {"acme/framework": "^11.0", "acme/electron": "^7.0"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	writePackage(t, "acme-electron", packageAtVersion("", "php acme base"))
	writePackage(t, "acme-electron@5", packageAtVersion("5", "php acme five"))

	fw, _ := GetFrameworkForDir("acme", project)
	if got := fw.Workers["native"].Command; got != "php acme five" {
		t.Errorf("worker command = %q, want the newest cached file below the wanted version", got)
	}
}

// A major that drops a command has to say so: the copy the declaration was
// lifted out of is still in the framework's version file.
func TestGetFrameworkForDir_packageRemovesEntries(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`}, "acme/electron")
	writePackage(t, "acme-electron", `package: acme/electron
removes:
  workers:
    - scheduler
  commands:
    - migrate
`)

	fw, _ := GetFrameworkForDir("acme", project)
	if _, has := fw.Workers["scheduler"]; has {
		t.Error("worker the package removed is still there")
	}
	if hasCommandNamed(fw.Commands, "migrate") {
		t.Error("command the package removed is still there")
	}
}

func TestPackageAppliesTo_scopes(t *testing.T) {
	cases := []struct {
		name  string
		scope []FrameworkPackageScope
		fw    Framework
		want  bool
	}{
		{"no scope applies everywhere", nil, Framework{Name: "acme", Version: "11"}, true},
		{"inclusive min", []FrameworkPackageScope{{Name: "acme", Min: "11"}}, Framework{Name: "acme", Version: "11"}, true},
		{"inclusive max", []FrameworkPackageScope{{Name: "acme", Max: "11"}}, Framework{Name: "acme", Version: "11"}, true},
		{"above max", []FrameworkPackageScope{{Name: "acme", Max: "11"}}, Framework{Name: "acme", Version: "12"}, false},
		{"detected version wins over the borrowed definition",
			[]FrameworkPackageScope{{Name: "acme", Min: "12"}},
			Framework{Name: "acme", Version: "11", DetectedVersion: "14"}, true},
		{"range needs a major to compare",
			[]FrameworkPackageScope{{Name: "acme", Min: "11"}}, Framework{Name: "acme"}, false},
		{"unbounded scope needs none",
			[]FrameworkPackageScope{{Name: "acme"}}, Framework{Name: "acme"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg := &FrameworkPackage{Package: "acme/electron", Frameworks: tc.scope}
			if got := pkg.AppliesTo(&tc.fw); got != tc.want {
				t.Errorf("AppliesTo = %v, want %v", got, tc.want)
			}
		})
	}
}

// The package layer asks whether the project has the package, and a project can
// have one it never named: a framework that replaces its own split packages is
// the only entry composer writes, so the manifest alone shows nothing.
func TestGetFrameworkForDir_packageMergedFromTheLock(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron"}`})
	writePackage(t, "acme-electron", electronPackage)
	writeLock(t, project, `{"packages": [{"name": "acme/framework", "version": "v11.2.0",
	  "replace": {"acme/electron": "self.version"}}]}`)

	fw, _ := GetFrameworkForDir("acme", project)
	if _, has := fw.Workers["native"]; !has {
		t.Error("a package the lock installed must contribute its worker")
	}
}

// Which file serves the project follows the version composer resolved, not the
// constraint, which a project tracking a branch does not have one of.
func TestGetFrameworkForDir_packageVersionPrefersTheLock(t *testing.T) {
	_, project := packageSandbox(t, []string{`{"name":"acme/electron","versions":["5","6"],"latest":"6"}`})
	if err := os.WriteFile(filepath.Join(project, "composer.json"),
		[]byte(`{"require": {"acme/framework": "^11.0", "acme/electron": "*"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	writeLock(t, project, `{"packages": [{"name": "acme/electron", "version": "5.4.0"}]}`)
	writePackage(t, "acme-electron", packageAtVersion("", "php acme base"))
	writePackage(t, "acme-electron@5", packageAtVersion("5", "php acme five"))
	writePackage(t, "acme-electron@6", packageAtVersion("6", "php acme six"))

	fw, _ := GetFrameworkForDir("acme", project)
	if got := fw.Workers["native"].Command; got != "php acme five" {
		t.Errorf("worker command = %q, want the file for the locked major", got)
	}
}

// A package's services are kept apart from the framework's own suggestions: the
// project requiring the package is evidence it uses them, so they are ticked
// where the framework's are only offered. Each is kept, in order, carrying the
// package it came from, for PickPackageSuggestions to narrow.
func TestApplyPackage_suggestServices(t *testing.T) {
	fw := &Framework{Name: "drupal", SuggestServices: []ServiceSuggestion{{Name: "solr"}, {Name: "memcached"}}}
	applyPackage(fw, &FrameworkPackage{Package: "drupal/search_api_solr", SuggestServices: []ServiceSuggestion{{Name: "solr", Reason: "Search backend"}}})
	applyPackage(fw, &FrameworkPackage{Package: "drupal/search_api_solr_extra", SuggestServices: []ServiceSuggestion{{Name: "solr"}}})

	if len(fw.SuggestServices) != 2 {
		t.Errorf("framework suggestions changed: %v", fw.SuggestServices)
	}
	want := []ServiceSuggestion{
		{Name: "solr", Reason: "Search backend", Package: "drupal/search_api_solr"},
		{Name: "solr", Package: "drupal/search_api_solr_extra"},
	}
	if !slices.Equal(fw.PackageServices, want) {
		t.Errorf("package services = %v, want %v", fw.PackageServices, want)
	}
}

func TestFrameworkPackage_suggestServicesYAML(t *testing.T) {
	var pkg FrameworkPackage
	src := "package: drupal/search_api_solr\nsuggest_services:\n  - name: solr\n    reason: Search backend\n"
	if err := yaml.Unmarshal([]byte(src), &pkg); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pkg.SuggestServices, []ServiceSuggestion{{Name: "solr", Reason: "Search backend"}}) {
		t.Errorf("suggest_services not bound: %v", pkg.SuggestServices)
	}
}
