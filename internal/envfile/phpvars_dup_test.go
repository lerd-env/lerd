package envfile

import (
	"os"
	"strings"
	"testing"
)

// PHP runs a file top to bottom, so the last assignment to a path is the one
// the application ends up with. Writing to the first left lerd's value shadowed
// by whatever came after it, and the site went on using the old database while
// every surface reported the new one.
func TestApplyPhpVarsUpdates_writesTheAssignmentThatWins(t *testing.T) {
	path := writeSettings(t, "<?php\n$databases['default']['default'] = ['host' => 'first'];\n$databases['default']['default'] = ['host' => 'second'];\n")

	if err := ApplyPhpVarsUpdates(path, map[string]string{"databases.default.default.host": "lerd-mysql"}); err != nil {
		t.Fatalf("ApplyPhpVarsUpdates: %v", err)
	}

	// Reading applies the assignments in order, so this is the value PHP would
	// see, and it has to be the one lerd wrote.
	if got := mustRead(t, path)["databases.default.default.host"]; got != "lerd-mysql" {
		t.Errorf("effective host = %q, want lerd-mysql", got)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "'second'") {
		t.Errorf("the winning assignment was left stale:\n%s", body)
	}
}

// A later leaf assignment overrides an earlier whole-array one, so that leaf is
// what has to change.
func TestApplyPhpVarsUpdates_writesALaterLeafOverAnEarlierArray(t *testing.T) {
	path := writeSettings(t, "<?php\n$databases['default']['default'] = ['host' => 'first', 'port' => 3306];\n$databases['default']['default']['host'] = 'override';\n")

	if err := ApplyPhpVarsUpdates(path, map[string]string{"databases.default.default.host": "lerd-mysql"}); err != nil {
		t.Fatalf("ApplyPhpVarsUpdates: %v", err)
	}

	vals := mustRead(t, path)
	if vals["databases.default.default.host"] != "lerd-mysql" {
		t.Errorf("effective host = %q, want lerd-mysql", vals["databases.default.default.host"])
	}
	if vals["databases.default.default.port"] != "3306" {
		t.Errorf("port = %q, want the 3306 the earlier assignment set", vals["databases.default.default.port"])
	}
}

func mustRead(t *testing.T, path string) map[string]string {
	t.Helper()
	vals, err := ReadPhpVars(path)
	if err != nil {
		t.Fatalf("ReadPhpVars: %v", err)
	}
	return vals
}
