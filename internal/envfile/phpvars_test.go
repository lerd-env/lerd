package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// drupalSettings is the shape Drupal's own installer leaves behind: hundreds of
// lines of commented guidance, then the assignments that actually run.
const drupalSettings = `<?php

/**
 * @file
 * Drupal site-specific configuration file.
 *
 * $databases['default']['default'] = [ ... ];  // an example inside a comment
 */

$settings['update_free_access'] = FALSE;

$databases = [];

$databases['default']['default'] = array (
  'prefix' => '',
  'database' => 'sites/default/files/.ht.sqlite',
  'driver' => 'sqlite',
  'namespace' => 'Drupal\\sqlite\\Driver\\Database\\sqlite',
  'autoload' => 'core/modules/sqlite/src/Driver/Database/sqlite/',
);
$settings['config_sync_directory'] = 'sites/default/files/sync';
`

func writeSettings(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.php")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Drupal addresses its database through top-level assignments, not a returned
// array, so the keys have to come out of the statements themselves.
func TestReadPhpVars(t *testing.T) {
	vals, err := ReadPhpVars(writeSettings(t, drupalSettings))
	if err != nil {
		t.Fatalf("ReadPhpVars: %v", err)
	}
	cases := map[string]string{
		"databases.default.default.database": "sites/default/files/.ht.sqlite",
		"databases.default.default.driver":   "sqlite",
		"settings.update_free_access":        "false",
		"settings.config_sync_directory":     "sites/default/files/sync",
	}
	for k, want := range cases {
		if vals[k] != want {
			t.Errorf("%s = %q, want %q", k, vals[k], want)
		}
	}
}

// An assignment written inside a comment is documentation, not configuration.
func TestReadPhpVars_ignoresCommentedExamples(t *testing.T) {
	vals, err := ReadPhpVars(writeSettings(t, "<?php\n// $databases['default']['default'] = ['driver' => 'mysql'];\n$settings['x'] = 'y';\n"))
	if err != nil {
		t.Fatalf("ReadPhpVars: %v", err)
	}
	if _, found := vals["databases.default.default.driver"]; found {
		t.Errorf("a commented assignment was read as configuration: %v", vals)
	}
}

// Writing has to change the values and leave everything else exactly as it was:
// settings.php is Drupal's file, and it is mostly guidance a user may have
// edited.
func TestApplyPhpVarsUpdates_rewritesInPlace(t *testing.T) {
	path := writeSettings(t, drupalSettings)

	err := ApplyPhpVarsUpdates(path, map[string]string{
		"databases.default.default.driver":    "mysql",
		"databases.default.default.database":  "drupal",
		"databases.default.default.host":      "lerd-mysql",
		"databases.default.default.port":      "3306",
		"databases.default.default.username":  "root",
		"databases.default.default.password":  "lerd",
		"databases.default.default.namespace": `Drupal\mysql\Driver\Database\mysql`,
	})
	if err != nil {
		t.Fatalf("ApplyPhpVarsUpdates: %v", err)
	}

	vals, err := ReadPhpVars(path)
	if err != nil {
		t.Fatalf("ReadPhpVars: %v", err)
	}
	for k, want := range map[string]string{
		"databases.default.default.driver":   "mysql",
		"databases.default.default.database": "drupal",
		"databases.default.default.host":     "lerd-mysql",
		"databases.default.default.port":     "3306",
		"databases.default.default.username": "root",
	} {
		if vals[k] != want {
			t.Errorf("%s = %q, want %q", k, vals[k], want)
		}
	}
	// The rest of Drupal's file survives untouched.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{
		"@file",
		"Drupal site-specific configuration file.",
		"$settings['update_free_access'] = FALSE;",
		"$settings['config_sync_directory'] = 'sites/default/files/sync';",
	} {
		if !strings.Contains(string(body), keep) {
			t.Errorf("rewrite dropped %q:\n%s", keep, body)
		}
	}
	// The prefix Drupal set and lerd said nothing about is still there.
	if vals["databases.default.default.prefix"] != "" {
		t.Errorf("prefix = %q, want the empty string Drupal wrote", vals["databases.default.default.prefix"])
	}
}

// A key no assignment covers is appended as its own statement, so a project
// missing a setting entirely still ends up with it.
func TestApplyPhpVarsUpdates_appendsAKeyNoAssignmentCovers(t *testing.T) {
	path := writeSettings(t, "<?php\n$settings['x'] = 'y';\n")

	if err := ApplyPhpVarsUpdates(path, map[string]string{"databases.default.default.host": "lerd-mysql"}); err != nil {
		t.Fatalf("ApplyPhpVarsUpdates: %v", err)
	}

	vals, _ := ReadPhpVars(path)
	if vals["databases.default.default.host"] != "lerd-mysql" {
		t.Errorf("host = %q, want lerd-mysql", vals["databases.default.default.host"])
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "$databases['default']['default']['host'] = 'lerd-mysql';") {
		t.Errorf("appended statement not as expected:\n%s", body)
	}
}

// An update that changes nothing must not rewrite the file, or every env sync
// churns a file the user is watching in an editor.
func TestApplyPhpVarsUpdates_skipsAnUnchangedWrite(t *testing.T) {
	path := writeSettings(t, drupalSettings)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := ApplyPhpVarsUpdates(path, map[string]string{"databases.default.default.driver": "sqlite"}); err != nil {
		t.Fatalf("ApplyPhpVarsUpdates: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an update that changes nothing rewrote the file")
	}
}

// An int in the file stays an int, the same rule the returned-array format
// follows, so a port does not turn into a quoted string.
func TestApplyPhpVarsUpdates_keepsScalarTypes(t *testing.T) {
	path := writeSettings(t, "<?php\n$databases['default']['default'] = ['port' => 3306, 'host' => 'localhost'];\n")

	if err := ApplyPhpVarsUpdates(path, map[string]string{
		"databases.default.default.port": "3307",
		"databases.default.default.host": "lerd-mysql",
	}); err != nil {
		t.Fatalf("ApplyPhpVarsUpdates: %v", err)
	}

	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "'port' => 3307") {
		t.Errorf("port lost its type:\n%s", body)
	}
	if !strings.Contains(string(body), "'host' => 'lerd-mysql'") {
		t.Errorf("host not written as a string:\n%s", body)
	}
}

// The format joins the others, so everything that reads an env file by name
// reads this one too.
func TestReader_readsPhpVars(t *testing.T) {
	path := writeSettings(t, drupalSettings)
	if got := Reader(path, "php-vars")("databases.default.default.database"); got != "sites/default/files/.ht.sqlite" {
		t.Errorf("Reader = %q, want the sqlite path", got)
	}
	if got := Reader(path, "php-vars")("databases.default.default.driver"); got != "sqlite" {
		t.Errorf("Reader = %q, want sqlite", got)
	}
}
