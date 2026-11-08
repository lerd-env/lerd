package envfile

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var fuzzKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// Whatever a site's .env already holds, a value lerd writes must read back
// unchanged, since service wiring trusts it on the next read.
func FuzzApplyUpdatesRoundTrip(f *testing.F) {
	f.Add("APP_NAME=Laravel\nDB_HOST=127.0.0.1\n", "DB_HOST", "lerd-mysql")
	f.Add("# DB_HOST=old\n", "DB_HOST", "lerd-postgres-18")
	f.Add("DB_PASSWORD=\"secret\"\nDB_PASSWORD=dup\n", "DB_PASSWORD", "p@ss=word")
	f.Add("", "REDIS_PORT", "6379")
	f.Add("  MAIL_HOST = smtp\r\nexport X=1", "MAIL_HOST", "lerd-mailpit")
	f.Add("<?php\n$databases = [];\n", "DB_HOST", "lerd-mysql")

	f.Fuzz(func(t *testing.T, content, key, value string) {
		if !fuzzKey.MatchString(key) || strings.ContainsAny(value, "\n\r") ||
			value != strings.Trim(strings.TrimSpace(value), `"'`) {
			t.Skip("outside what the unquoted .env writer can represent")
		}
		path := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		err := ApplyUpdates(path, map[string]string{key: value})
		if isPHPSource([]byte(content)) {
			if err == nil {
				t.Fatalf("wrote dotenv lines into PHP source %q", content)
			}
			return
		}
		if err != nil {
			t.Fatalf("ApplyUpdates: %v", err)
		}
		if got := ReadValues(path)[key]; got != value {
			t.Fatalf("wrote %s=%q over %q, read back %q", key, value, content, got)
		}
	})
}

// A project's PHP config file is arbitrary text; reading it must never panic.
func FuzzParsePhpReturn(f *testing.F) {
	f.Add("<?php\nreturn ['db' => ['host' => 'localhost', 'port' => 3306]];\n")
	f.Add("<?php return array('a' => \"x\\\"y\", 'b' => [1, 2, [true, null]]);")
	f.Add("<?php\n// return ['no'];\n$x = 'return';\nreturn [ 'k' => env('X', 'd') ];")
	f.Add("<?php return [")

	f.Fuzz(func(t *testing.T, src string) {
		root, err := parsePhpReturn(src)
		if err != nil || root == nil {
			return
		}
		flatten("", root, map[string]string{})
	})
}
