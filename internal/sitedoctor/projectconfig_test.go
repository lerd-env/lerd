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

// Settings coming from the untracked override file are named in the report, so
// a domain or an isolated database that is not in .lerd.yaml is still visible.
func TestCheckProjectConfig_NamesLocalOverrides(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.yaml", "php_version: \"8.4\"\n")
	writeEnv(t, dir, ".lerd.local.yaml", "db_isolated: true\ndomains:\n  - acme-branch\n")
	c, ok := checkProjectConfig(dir, nil)
	if !ok {
		t.Fatal("expected a project_config check")
	}
	if c.Status == StatusFail {
		t.Fatalf("status = %q, want a non-failing check: %s", c.Status, c.Detail)
	}
	for _, want := range []string{".lerd.local.yaml", "db_isolated", "domains"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("detail %q does not mention %q", c.Detail, want)
		}
	}
}

// An override file on its own still gets validated; skipping on the absence of
// .lerd.yaml would hide a project configured entirely from the local file.
func TestCheckProjectConfig_LocalFileAlone(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, ".lerd.local.yaml", "php_version: \"8,5\"\n")
	c, ok := checkProjectConfig(dir, nil)
	if !ok || c.Status != StatusFail {
		t.Fatalf("got ok=%v status=%q, want a failing check", ok, c.Status)
	}
}
