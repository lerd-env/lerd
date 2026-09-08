package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// writeStoreIndex publishes a catalogue to the cached index the wizard reads.
func writeStoreIndex(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(config.StoreFrameworksDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(body)) {
		t.Fatalf("test index is not valid json: %s", body)
	}
	if err := os.WriteFile(config.StoreIndexFile(), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// The wizard offers what the store publishes, in the order the store lists it,
// with each framework's majors newest first so the default lands on the current
// release.
func TestScaffoldCatalogue_FollowsThePublishedOrder(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	writeStoreIndex(t, `{"frameworks":[
	  {"name":"laravel","label":"Laravel","versions":["13","12","11","10"],"latest":"13"},
	  {"name":"drupal","label":"Drupal","versions":["10","11"],"latest":"11"}
	]}`)

	got := scaffoldCatalogue()
	if got[0].Name != "laravel" || got[1].Name != "drupal" {
		t.Errorf("order starts %s, %s, want laravel, drupal", got[0].Name, got[1].Name)
	}
	if got[0].Label != "Laravel" {
		t.Errorf("label = %q, want Laravel", got[0].Label)
	}
	drupal := scaffoldChoiceByName(got, "drupal")
	if drupal.Versions[0] != "11" {
		t.Errorf("drupal versions = %v, want 11 first", drupal.Versions)
	}
	if got[0].Latest != "13" {
		t.Errorf("laravel latest = %q, want 13", got[0].Latest)
	}
}

// catalogueNames lists the entries in offered order.
func catalogueNames(catalogue []scaffoldChoice) []string {
	names := make([]string, 0, len(catalogue))
	for _, c := range catalogue {
		names = append(names, c.Name)
	}
	return names
}

// Not every published framework can start a project: an install that is not a
// composer create-project publishes no create command. Offering one and then
// refusing it after every question is worse than never listing it.
func TestScaffoldCatalogue_OmitsFrameworksThatCannotScaffold(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	writeStoreIndex(t, `{"frameworks":[
	  {"name":"laravel","label":"Laravel","versions":["13"],"latest":"13"},
	  {"name":"wordpress","label":"WordPress","versions":["6","5"],"latest":"6"}
	]}`)
	for _, v := range []string{"5", "6"} {
		if err := config.SaveStoreFramework(&config.Framework{
			Name: "wordpress", Label: "WordPress", Version: v, PublicDir: ".",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.SaveStoreFramework(&config.Framework{
		Name: "laravel", Label: "Laravel", Version: "13", PublicDir: "public",
		Create: "composer create-project laravel/laravel",
	}); err != nil {
		t.Fatal(err)
	}

	got := scaffoldCatalogue()
	if scaffoldChoiceByName(got, "wordpress") != nil {
		t.Errorf("catalogue = %v, want wordpress left out: no definition can scaffold it",
			catalogueNames(got))
	}
	if scaffoldChoiceByName(got, "laravel") == nil {
		t.Errorf("catalogue = %v, want laravel offered", catalogueNames(got))
	}
}

// A framework the index publishes but this machine has not installed says
// nothing about whether it can scaffold, so it stays on offer and its definition
// is fetched when picked.
func TestScaffoldCatalogue_KeepsPublishedFrameworksItCannotInspect(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	writeStoreIndex(t, `{"frameworks":[
	  {"name":"tempest","label":"Tempest","versions":["3"],"latest":"3"}
	]}`)

	got := scaffoldCatalogue()
	if scaffoldChoiceByName(got, "tempest") == nil {
		t.Errorf("catalogue = %v, want the uninstalled tempest still offered",
			catalogueNames(got))
	}
}

// A framework whose newest major ships no project skeleton keeps the majors that
// do. Dropping the whole name would hide every version it can still scaffold,
// and offering the major it cannot only refuses the run after every question.
func TestScaffoldCatalogue_OffersOnlyTheMajorsThatScaffold(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	writeStoreIndex(t, `{"frameworks":[
	  {"name":"lumen","label":"Lumen","versions":["11","10"],"latest":"11"}
	]}`)
	if err := config.SaveStoreFramework(&config.Framework{
		Name: "lumen", Label: "Lumen", Version: "11", PublicDir: "public",
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveStoreFramework(&config.Framework{
		Name: "lumen", Label: "Lumen", Version: "10", PublicDir: "public",
		Create: "composer create-project laravel/lumen",
	}); err != nil {
		t.Fatal(err)
	}

	got := scaffoldCatalogue()
	lumen := scaffoldChoiceByName(got, "lumen")
	if lumen == nil {
		t.Fatalf("catalogue = %v, want lumen offered: major 10 scaffolds", catalogueNames(got))
	}
	if slices.Contains(lumen.Versions, "11") {
		t.Errorf("versions = %v, want 11 left out: it ships no skeleton", lumen.Versions)
	}
	if !slices.Contains(lumen.Versions, "10") {
		t.Errorf("versions = %v, want 10 offered", lumen.Versions)
	}
}

// A major this machine holds no definition for says nothing about whether it can
// scaffold, so the one installed major failing to is no reason to drop the rest.
func TestScaffoldCatalogue_KeepsMajorsItHasNotRead(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	writeStoreIndex(t, `{"frameworks":[
	  {"name":"lumen","label":"Lumen","versions":["11","10"],"latest":"11"}
	]}`)
	if err := config.SaveStoreFramework(&config.Framework{
		Name: "lumen", Label: "Lumen", Version: "11", PublicDir: "public",
	}); err != nil {
		t.Fatal(err)
	}

	got := scaffoldCatalogue()
	lumen := scaffoldChoiceByName(got, "lumen")
	if lumen == nil {
		t.Fatalf("catalogue = %v, want lumen offered: major 10 was never read", catalogueNames(got))
	}
	if !slices.Contains(lumen.Versions, "10") {
		t.Errorf("versions = %v, want the uninspected 10 offered", lumen.Versions)
	}
}

// Majors are compared as numbers. Sorted as strings, 9 outranks 12 and the
// wizard would default a Laravel project onto the older release.
func TestSortFrameworkVersionsDesc_IsNumeric(t *testing.T) {
	versions := []string{"9", "12", "10"}
	sortFrameworkVersionsDesc(versions)
	want := []string{"12", "10", "9"}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", versions, want)
		}
	}
}

// A framework installed here that the catalogue no longer lists is still
// offered, below the published ones, so a definition someone keeps locally does
// not vanish from the picker.
func TestScaffoldCatalogue_KeepsInstalledOnlyFrameworksLast(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	writeStoreIndex(t, `{"frameworks":[
	  {"name":"laravel","label":"Laravel","versions":["13"],"latest":"13"}
	]}`)
	if err := config.SaveStoreFramework(&config.Framework{
		Name: "mystack", Label: "My Stack", Version: "2", PublicDir: "public",
		Create: "composer create-project mystack/app",
	}); err != nil {
		t.Fatal(err)
	}

	got := scaffoldCatalogue()
	names := catalogueNames(got)
	if got[0].Name != "laravel" {
		t.Errorf("catalogue = %v, want the published laravel first", names)
	}

	local := scaffoldChoiceByName(got, "mystack")
	if local == nil {
		t.Fatalf("catalogue = %v, want the locally installed mystack offered", names)
	}
	if local.Label != "My Stack" {
		t.Errorf("label = %q, want My Stack", local.Label)
	}
	// Everything the index publishes sits above everything that only exists here,
	// so the picker opens on the store's own order.
	for i, name := range names {
		if name == "mystack" && i == 0 {
			t.Errorf("catalogue = %v, want mystack below the published entries", names)
		}
	}
	if local.Versions[0] != "2" {
		t.Errorf("mystack versions = %v, want its installed 2", local.Versions)
	}
}

// An install that has never reached the store still offers what it has, so the
// wizard works offline instead of falling silently back to one framework.
func TestScaffoldCatalogue_WorksWithNoIndex(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	for _, v := range []string{"12", "13"} {
		if err := config.SaveStoreFramework(&config.Framework{
			Name: "laravel", Label: "Laravel", Version: v, PublicDir: "public",
			Create: "composer create-project laravel/laravel",
		}); err != nil {
			t.Fatal(err)
		}
	}

	got := scaffoldCatalogue()
	choice := scaffoldChoiceByName(got, "laravel")
	if choice == nil {
		t.Fatalf("catalogue = %+v, want the installed laravel", got)
	}
	if len(choice.Versions) != 2 || choice.Versions[0] != "13" {
		t.Errorf("versions = %v, want 13 first of two", choice.Versions)
	}
	if choice.Latest != "13" {
		t.Errorf("latest = %q, want 13 inferred from what is installed", choice.Latest)
	}
}

// The question is only asked when the command was not told the answer, and never
// where there is no terminal to answer it.
func TestNewShouldAskFramework(t *testing.T) {
	cases := []struct {
		interactive, given, want bool
	}{
		{true, false, true},
		{true, true, false},
		{false, false, false},
		{false, true, false},
	}
	for _, tc := range cases {
		if got := newShouldAskFramework(tc.interactive, tc.given); got != tc.want {
			t.Errorf("newShouldAskFramework(%v, %v) = %v, want %v",
				tc.interactive, tc.given, got, tc.want)
		}
	}
}

// The version picker marks which major the store considers current, so picking
// an older one is a deliberate act rather than a guess.
func TestVersionSelectOptions_MarksLatest(t *testing.T) {
	opts := versionSelectOptions(scaffoldChoice{
		Name: "laravel", Label: "Laravel", Versions: []string{"13", "12"}, Latest: "13",
	})
	if len(opts) != 2 {
		t.Fatalf("options = %+v, want 2", opts)
	}
	if opts[0].Key != "13 (latest)" || opts[0].Value != "13" {
		t.Errorf("first option = %q/%q, want 13 (latest)/13", opts[0].Key, opts[0].Value)
	}
	if opts[1].Key != "12" {
		t.Errorf("second option = %q, want a bare 12", opts[1].Key)
	}
}

// The picker starts on the framework this command has always scaffolded, and on
// whatever is first when that one is not on offer at all.
func TestInitialScaffoldFramework(t *testing.T) {
	catalogue := []scaffoldChoice{
		{Name: "drupal", Label: "Drupal"},
		{Name: defaultScaffoldFramework, Label: "Default"},
	}
	if got := initialScaffoldFramework(catalogue); got != defaultScaffoldFramework {
		t.Errorf("initial = %q, want %q", got, defaultScaffoldFramework)
	}
	if got := initialScaffoldFramework(catalogue[:1]); got != "drupal" {
		t.Errorf("initial without the default = %q, want drupal", got)
	}
}

// A blank answer cannot become a directory, so the prompt has to reject it there
// rather than let the scaffold fail several steps later.
func TestValidateProjectName(t *testing.T) {
	for _, bad := range []string{"", "   ", "\t"} {
		if err := validateProjectName(bad); err == nil {
			t.Errorf("validateProjectName(%q) = nil, want an error", bad)
		}
	}
	for _, good := range []string{"myapp", "apps/myapp", "/abs/myapp"} {
		if err := validateProjectName(good); err != nil {
			t.Errorf("validateProjectName(%q) = %v, want nil", good, err)
		}
	}
}

// The chained steps read the working directory for themselves, so lerd new has
// to move into the project and put the shell back where it found it.
func TestInDirMovesAndRestores(t *testing.T) {
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	seen := ""
	if err := inDir(target, func() error {
		seen, _ = os.Getwd()
		return nil
	}); err != nil {
		t.Fatalf("inDir: %v", err)
	}
	if seen != target {
		t.Errorf("ran in %q, want %q", seen, target)
	}
	after, _ := os.Getwd()
	if after != before {
		t.Errorf("left the process in %q, want %q", after, before)
	}
}

// A failing step must not strand the process in the new project either.
func TestInDirRestoresAfterAnError(t *testing.T) {
	before, _ := os.Getwd()
	if err := inDir(t.TempDir(), func() error { return os.ErrInvalid }); err == nil {
		t.Fatal("inDir swallowed the error")
	}
	if after, _ := os.Getwd(); after != before {
		t.Errorf("left the process in %q, want %q", after, before)
	}
}
