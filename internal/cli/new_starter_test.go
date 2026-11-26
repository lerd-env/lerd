package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
)

const starterIndex = `{"frameworks":[
  {"name":"laravel","label":"Laravel","versions":["13","12","11","10"],"latest":"13"},
  {"name":"drupal","label":"Drupal","versions":["11"],"latest":"11"}
],"starters":[
  {"name":"filament","label":"Filament","description":"An admin panel","framework":{"name":"laravel","min":"11"}},
  {"name":"react","label":"React","framework":{"name":"laravel","min":"13"}}
]}`

func TestNewCmdParsesStarter(t *testing.T) {
	cmd := NewNewCmd()
	var starter string
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		starter, _ = c.Flags().GetString("starter")
		return nil
	}
	cmd.SetArgs([]string{"myapp", "--framework=laravel", "--starter=filament"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if starter != "filament" {
		t.Errorf("starter = %q, want filament", starter)
	}
}

// Starters belong to a framework, so a starter named without one has nothing to
// be looked up against, the same as a version.
func TestRunNewRejectsStarterWithoutFramework(t *testing.T) {
	err := runNew("myapp", "", "", "filament", nil)
	if err == nil || !strings.Contains(err.Error(), "--starter") {
		t.Fatalf("expected the starter flag to be refused without a framework, got %v", err)
	}
}

// The dashboard filters starters by the major picked, so each one carries the
// majors of its framework it supports rather than a range to interpret.
func TestFrameworkCatalogue_CarriesStartersWithTheMajorsTheySupport(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	writeStoreIndex(t, starterIndex)

	var laravel, drupal *FrameworkChoice
	for _, c := range FrameworkCatalogue() {
		switch c.Name {
		case "laravel":
			laravel = &c
		case "drupal":
			drupal = &c
		}
	}
	if laravel == nil || len(laravel.Starters) != 2 {
		t.Fatalf("laravel starters = %+v", laravel)
	}
	if got := strings.Join(laravel.Starters[0].Versions, ","); got != "13,12,11" {
		t.Errorf("filament versions = %s, want 13,12,11", got)
	}
	if laravel.Starters[0].Description != "An admin panel" {
		t.Errorf("description not carried: %+v", laravel.Starters[0])
	}
	if got := strings.Join(laravel.Starters[1].Versions, ","); got != "13" {
		t.Errorf("react versions = %s, want 13", got)
	}
	if drupal == nil || len(drupal.Starters) != 0 {
		t.Errorf("drupal should have no starters: %+v", drupal)
	}
}

// The question leads with no starter, so a plain project stays one keypress,
// and offers only what supports the major picked.
func TestStarterSelectOptions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	writeStoreIndex(t, starterIndex)

	choice := scaffoldChoiceByName(scaffoldCatalogue(), "laravel")
	opts := starterSelectOptions(*choice, "12")
	if len(opts) != 2 || opts[0].Value != "" || opts[1].Value != "filament" {
		t.Fatalf("options for 12 = %+v, want none then filament", opts)
	}
	if got := starterSelectOptions(*choice, "10"); got != nil {
		t.Errorf("a major no starter supports should not be asked about, got %+v", got)
	}
}

func TestScaffoldCreate_aStarterSkeletonReplacesTheFrameworks(t *testing.T) {
	fw := &config.Framework{Create: "composer create-project laravel/laravel"}
	if got := scaffoldCreate(fw, nil); got != fw.Create {
		t.Errorf("no starter = %q", got)
	}
	if got := scaffoldCreate(fw, &config.Starter{Require: []string{"filament/filament"}}); got != fw.Create {
		t.Errorf("a starter that only requires keeps the framework's create, got %q", got)
	}
	kit := &config.Starter{Create: "composer create-project laravel/react-starter-kit"}
	if got := scaffoldCreate(fw, kit); got != kit.Create {
		t.Errorf("a starter skeleton = %q, want %q", got, kit.Create)
	}
}
