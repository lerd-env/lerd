package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
)

// runNewCmdVersion parses args through the real command and reports both
// framework flags, so the test only observes what flag parsing produced.
func runNewCmdVersion(t *testing.T, args ...string) (framework, version string, err error) {
	t.Helper()
	cmd := NewNewCmd()
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		framework, _ = c.Flags().GetString("framework")
		version, _ = c.Flags().GetString("framework-version")
		return nil
	}
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	err = cmd.Execute()
	return
}

// A caller with no terminal (the dashboard wizard, a script) has to be able to
// name the major it wants, since the question that would ask never runs.
func TestNewCmdParsesFrameworkVersion(t *testing.T) {
	framework, version, err := runNewCmdVersion(t, "myapp", "--framework=laravel", "--framework-version=11")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if framework != "laravel" || version != "11" {
		t.Errorf("framework = %q, version = %q, want laravel/11", framework, version)
	}
}

// A version with no framework to apply it to is a mistake worth reporting: the
// wizard would ask which framework and quietly drop the version that was typed.
func TestNewVersionNeedsFramework(t *testing.T) {
	if !newVersionNeedsFramework("", "11") {
		t.Error("a version with no framework should be rejected")
	}
	if newVersionNeedsFramework("laravel", "11") {
		t.Error("a version alongside a framework is fine")
	}
	if newVersionNeedsFramework("", "") {
		t.Error("neither flag given is fine")
	}
}

// The rejection reaches the user as an error rather than a silently ignored
// flag, and names the flag it needs.
func TestRunNewRejectsVersionWithoutFramework(t *testing.T) {
	err := runNew("myapp", "", "11", nil)
	if err == nil {
		t.Fatal("expected an error for --framework-version with no --framework")
	}
	if !strings.Contains(err.Error(), "--framework") {
		t.Errorf("error = %v, want it to name the missing flag", err)
	}
}

// A major that ships no project skeleton while others do is not a broken
// definition, and telling the user to add a create field sends them to fix a
// file that is right. It names the run that would work instead.
func TestScaffoldUnavailableError_PointsAtTheMajorsThatScaffold(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

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

	err := scaffoldUnavailableError("lumen", "11")
	if !strings.Contains(err.Error(), "--framework-version") {
		t.Errorf("error = %v, want it to name the flag to drop", err)
	}
	if strings.Contains(err.Error(), "create") {
		t.Errorf("error = %v, want no advice to edit the definition", err)
	}
}

// A framework no major of which can scaffold is a definition question after all,
// and the message stays the one that says so.
func TestScaffoldUnavailableError_KeepsTheDefinitionAdvice(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	if err := config.SaveStoreFramework(&config.Framework{
		Name: "wordpress", Label: "WordPress", Version: "6", PublicDir: ".",
	}); err != nil {
		t.Fatal(err)
	}

	err := scaffoldUnavailableError("wordpress", "6")
	if !strings.Contains(err.Error(), "create") {
		t.Errorf("error = %v, want the definition advice", err)
	}
}
