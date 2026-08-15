package sitedoctor

import (
	"strings"
	"testing"
)

// A project with no .lerd.yaml has nothing to validate, so the check stays out
// of the report rather than reporting a pass it never made.
func TestCheckProjectConfig_AbsentFile(t *testing.T) {
	if _, ok := checkProjectConfig(t.TempDir(), nil); ok {
		t.Error("expected no project_config check without a .lerd.yaml")
	}
}

func TestCheckProjectConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", "php_version: \"8.4\"\n")
	c, ok := checkProjectConfig(dir, nil)
	if !ok || c.Status == StatusFail {
		t.Fatalf("got ok=%v status=%q detail=%q, want a non-failing check", ok, c.Status, c.Detail)
	}
}

// The findings `lerd check` used to print now land in the site report, so the
// fold doesn't lose the validation it was the only source of.
func TestCheckProjectConfig_ReportsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", "php_version: \"8,5\"\ncommands:\n  - name: seed\n")
	c, ok := checkProjectConfig(dir, nil)
	if !ok || c.Status != StatusFail {
		t.Fatalf("got ok=%v status=%q, want a failing check", ok, c.Status)
	}
	for _, want := range []string{"php_version", "seed"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("detail %q does not mention %q", c.Detail, want)
		}
	}
}

// A worker the project defines itself is defined, whether or not the site runs a
// custom container: only the container branch used to look at custom_workers, so
// a plain site's own worker was reported as having no definition to match.
func TestValidateProjectConfig_CustomWorkerOnAPlainSite(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", "workers:\n  - reverb\ncustom_workers:\n  reverb:\n    command: php artisan reverb:start\n")
	problems, warnings := ValidateProjectConfig(dir, nil)
	if len(problems) != 0 || len(warnings) != 0 {
		t.Errorf("got problems=%v warnings=%v, want none", problems, warnings)
	}
}

// A command with no label still works, so it is a warning rather than a problem.
func TestValidateProjectConfig_LabellessCommandWarnsOnly(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", "commands:\n  - name: seed\n    command: php artisan db:seed\n")
	problems, warnings := ValidateProjectConfig(dir, nil)
	if len(problems) != 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "label") {
		t.Errorf("warnings: got %v, want one about the empty label", warnings)
	}
}
