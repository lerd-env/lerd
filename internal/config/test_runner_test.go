package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	unitPackage = "package: acme/unit\ntest:\n  command: php vendor/bin/unit\n  filter: --filter={filter}\n"
	nicePackage = "package: acme/nice\ntest:\n  command: php vendor/bin/nice run\n  filter: --only {filter}\n  over:\n    - acme/unit\n"
)

func testRunnerSandbox(t *testing.T, require ...string) (store, project string) {
	t.Helper()
	// unit is listed first, so nice only wins by what it declares.
	store, project = packageSandbox(t, []string{`{"name":"acme/unit"}`, `{"name":"acme/nice"}`}, require...)
	writePackage(t, "acme-unit", unitPackage)
	writePackage(t, "acme-nice", nicePackage)
	return store, project
}

func mustRunner(t *testing.T, project string) *TestRunner {
	t.Helper()
	r, err := TestRunnerForDir(project)
	if err != nil || r == nil {
		t.Fatalf("runner = %v, err = %v", r, err)
	}
	return r
}

func TestTestRunnerForDir_packageDeclaresIt(t *testing.T) {
	_, project := testRunnerSandbox(t, "acme/unit")
	if got := mustRunner(t, project).Command; got != "php vendor/bin/unit" {
		t.Errorf("command = %q", got)
	}
}

func TestTestRunnerForDir_packageBuiltOnAnotherWins(t *testing.T) {
	_, project := testRunnerSandbox(t, "acme/unit", "acme/nice")
	if got := mustRunner(t, project).Command; got != "php vendor/bin/nice run" {
		t.Errorf("command = %q", got)
	}
}

func TestTestRunnerForDir_frameworkWinsWhenItsCheckPasses(t *testing.T) {
	store, project := testRunnerSandbox(t, "acme/unit")
	def := filepath.Join(store, "acme@11.yaml")
	b, err := os.ReadFile(def)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b) + "test:\n  command: php acme test\n  check:\n    composer: acme/printer\n"
	if err := os.WriteFile(def, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustRunner(t, project).Command; got != "php vendor/bin/unit" {
		t.Errorf("check fails, command = %q", got)
	}
	if err := os.WriteFile(filepath.Join(project, "composer.json"),
		[]byte(`{"require":{"acme/framework":"^11.0","acme/unit":"^1.0","acme/printer":"^1.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustRunner(t, project).Command; got != "php acme test" {
		t.Errorf("check passes, command = %q", got)
	}
}

func TestTestRunnerForDir_worksWithoutAFramework(t *testing.T) {
	testRunnerSandbox(t)
	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "composer.json"), []byte(`{"require-dev":{"acme/unit":"^1.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustRunner(t, plain).Command; got != "php vendor/bin/unit" {
		t.Errorf("command = %q", got)
	}
}

func TestTestRunnerForDir_noneDeclared(t *testing.T) {
	_, project := testRunnerSandbox(t)
	if r, err := TestRunnerForDir(project); r != nil || err != nil {
		t.Errorf("runner = %v, err = %v, want neither", r, err)
	}
}

func TestTestRunnerForDir_refusesACommandThatIsNotPHP(t *testing.T) {
	_, project := testRunnerSandbox(t, "acme/unit")
	writePackage(t, "acme-unit", "package: acme/unit\ntest:\n  command: sh -c unit\n")
	if _, err := TestRunnerForDir(project); err == nil || !strings.Contains(err.Error(), "sh -c unit") {
		t.Errorf("err = %v, want it to name the command", err)
	}
}

// A cloned repo may not choose what lerd runs for its tests.
func TestSanitizeProjectFrameworkDef_dropsTest(t *testing.T) {
	safe := SanitizeProjectFrameworkDef(&Framework{Name: "x", Test: &TestRunner{Command: "php evil"}})
	if safe.Test != nil {
		t.Error("a project definition kept its test command")
	}
}

func TestTestRunnerArgv(t *testing.T) {
	unit := TestRunner{Command: "php vendor/bin/unit", Filter: "--filter={filter}"}
	nice := TestRunner{Command: "php vendor/bin/nice run", Filter: "--only {filter}"}
	for _, c := range []struct {
		r            TestRunner
		file, filter string
		want         []string
	}{
		{unit, "", "", []string{"php", "vendor/bin/unit"}},
		{unit, "tests/A.php", "it adds $(x)", []string{"php", "vendor/bin/unit", "--filter=it adds $(x)", "tests/A.php"}},
		{nice, "", "two words", []string{"php", "vendor/bin/nice", "run", "--only", "two words"}},
	} {
		got, err := c.r.Argv(c.file, c.filter)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("argv = %q, %v, want %q", got, err, c.want)
		}
	}
	if _, err := (&TestRunner{Command: "php x"}).Argv("", "a"); err == nil {
		t.Error("a filter the runner cannot take was accepted")
	}
}
