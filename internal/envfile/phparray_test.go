package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const magentoEnvPHP = `<?php
return [
    'backend' => [
        'frontName' => 'admin'
    ],
    'db' => [
        'connection' => [
            'default' => [
                'host' => 'localhost',
                'dbname' => 'magento',
                'username' => 'root',
                'password' => 'secret',
                'active' => '1'
            ]
        ],
        'table_prefix' => ''
    ],
    'x-frame-options' => 'SAMEORIGIN',
    'MAGE_MODE' => 'default',
    'cache_types' => [
        'config' => 1,
        'layout' => 0
    ],
    'downloadable_domains' => [
        'magento.test'
    ],
    'install' => [
        'date' => 'Thu, 09 Jul 2026 10:00:00 +0000'
    ]
];
`

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "env.php")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadPhpArrayFlattensNestedKeys(t *testing.T) {
	got, err := ReadPhpArray(writeTemp(t, magentoEnvPHP))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"backend.frontName":              "admin",
		"db.connection.default.host":     "localhost",
		"db.connection.default.dbname":   "magento",
		"db.connection.default.username": "root",
		"db.connection.default.password": "secret",
		"db.connection.default.active":   "1",
		"db.table_prefix":                "",
		"x-frame-options":                "SAMEORIGIN",
		"MAGE_MODE":                      "default",
		"cache_types.config":             "1",
		"cache_types.layout":             "0",
		"downloadable_domains.0":         "magento.test",
		"install.date":                   "Thu, 09 Jul 2026 10:00:00 +0000",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d keys, want %d: %v", len(got), len(want), got)
	}
}

func TestReadPhpArrayHandlesSyntaxVariants(t *testing.T) {
	src := `<?php
// a comment with ] and '
# hash comment
/* block ' comment */
return array(
    "double" => "quoted",
    'esc' => 'it\'s',
    'num' => 3306,
    'yes' => true,
    'no' => false,
    'nil' => null,
    'trailing' => array('a' => 1,),
);
`
	got, err := ReadPhpArray(writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"double": "quoted", "esc": "it's", "num": "3306",
		"yes": "true", "no": "false", "nil": "", "trailing.a": "1",
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}

// A `return` sitting inside a string literal before the real return statement
// must not be mistaken for it, or the scanner parses the wrong value.
func TestReadPhpArraySkipsReturnInsideString(t *testing.T) {
	src := `<?php
$note = 'remember to return the array';
return [
    'db' => ['host' => 'localhost'],
];
`
	got, err := ReadPhpArray(writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if got["db.host"] != "localhost" {
		t.Fatalf("scanner matched a return inside a string: %v", got)
	}
}

func TestReadPhpArrayMissingOrEmptyFile(t *testing.T) {
	if _, err := ReadPhpArray(filepath.Join(t.TempDir(), "nope.php")); err == nil {
		t.Error("missing file should error")
	}
	got, err := ReadPhpArray(writeTemp(t, "<?php\n"))
	if err != nil {
		t.Fatalf("file with no return should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestApplyPhpArrayUpdatesRewritesNestedValue(t *testing.T) {
	p := writeTemp(t, magentoEnvPHP)
	err := ApplyPhpArrayUpdates(p, map[string]string{
		"db.connection.default.host":     "lerd-mysql",
		"db.connection.default.dbname":   "magento_test",
		"db.connection.default.password": "lerd",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadPhpArray(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["db.connection.default.host"] != "lerd-mysql" {
		t.Errorf("host = %q", got["db.connection.default.host"])
	}
	if got["db.connection.default.dbname"] != "magento_test" {
		t.Errorf("dbname = %q", got["db.connection.default.dbname"])
	}
	// Untouched keys survive.
	if got["backend.frontName"] != "admin" || got["install.date"] == "" {
		t.Errorf("unrelated keys lost: %v", got)
	}

	// The php-const writer appends a dead define() after the return. This one
	// must never do that.
	body, _ := os.ReadFile(p)
	if strings.Contains(string(body), "define(") {
		t.Errorf("appended dead code:\n%s", body)
	}
	if strings.Count(string(body), "return") != 1 {
		t.Errorf("expected exactly one return:\n%s", body)
	}
}

func TestApplyPhpArrayUpdatesCreatesMissingPath(t *testing.T) {
	p := writeTemp(t, magentoEnvPHP)
	err := ApplyPhpArrayUpdates(p, map[string]string{
		"system.default.catalog.search.engine":                     "opensearch",
		"system.default.catalog.search.opensearch_server_hostname": "lerd-opensearch",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := ReadPhpArray(p)
	if got["system.default.catalog.search.engine"] != "opensearch" {
		t.Fatalf("engine = %q\n", got["system.default.catalog.search.engine"])
	}
	if got["system.default.catalog.search.opensearch_server_hostname"] != "lerd-opensearch" {
		t.Fatalf("hostname = %q", got["system.default.catalog.search.opensearch_server_hostname"])
	}
	if got["db.connection.default.host"] != "localhost" {
		t.Error("existing keys clobbered")
	}
}

// Writing then reading then writing must converge, or `lerd env` would churn
// the file on every run.
func TestApplyPhpArrayUpdatesIsIdempotent(t *testing.T) {
	p := writeTemp(t, magentoEnvPHP)
	up := map[string]string{"db.connection.default.host": "lerd-mysql"}
	if err := ApplyPhpArrayUpdates(p, up); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(p)
	if err := ApplyPhpArrayUpdates(p, up); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Errorf("not idempotent:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// Types are preserved: an int stays an int, a bool stays a bool, so Magento's
// own config reader sees what it expects.
func TestApplyPhpArrayUpdatesPreservesScalarTypes(t *testing.T) {
	p := writeTemp(t, "<?php\nreturn [\n    'port' => 3306,\n    'on' => true,\n    'name' => 'x',\n];\n")
	if err := ApplyPhpArrayUpdates(p, map[string]string{"port": "3307", "on": "false", "name": "y"}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(p)
	s := string(body)
	if !strings.Contains(s, "'port' => 3307,") {
		t.Errorf("port should stay an int:\n%s", s)
	}
	if !strings.Contains(s, "'on' => false,") {
		t.Errorf("bool should stay a bool:\n%s", s)
	}
	if !strings.Contains(s, "'name' => 'y',") {
		t.Errorf("string should stay quoted:\n%s", s)
	}
}

func TestApplyPhpArrayUpdatesCreatesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "app", "etc", "env.php")
	if err := ApplyPhpArrayUpdates(p, map[string]string{"db.connection.default.host": "lerd-mysql"}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPhpArray(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["db.connection.default.host"] != "lerd-mysql" {
		t.Fatalf("got %v", got)
	}
	body, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(body), "<?php") {
		t.Errorf("missing php open tag:\n%s", body)
	}
}

// A value containing a quote or backslash must round-trip, not break the file.
func TestApplyPhpArrayUpdatesEscapesValues(t *testing.T) {
	p := writeTemp(t, "<?php\nreturn ['a' => 'x'];\n")
	if err := ApplyPhpArrayUpdates(p, map[string]string{"a": `it's a \ back`}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPhpArray(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != `it's a \ back` {
		t.Fatalf("got %q", got["a"])
	}
}

func TestPhpArrayHandlesFloats(t *testing.T) {
	p := writeTemp(t, "<?php\nreturn ['ratio' => 1.5, 'neg' => -2.25, 'sci' => 1.0e-5, 'i' => 7];\n")
	got, err := ReadPhpArray(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["ratio"] != "1.5" || got["neg"] != "-2.25" || got["i"] != "7" {
		t.Fatalf("got %v", got)
	}
	if err := ApplyPhpArrayUpdates(p, map[string]string{"ratio": "2.75"}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(p)
	if !strings.Contains(string(body), "'ratio' => 2.75,") {
		t.Fatalf("float not preserved:\n%s", body)
	}
}

func TestReaderDispatchesOnFormat(t *testing.T) {
	arr := writeTemp(t, "<?php\nreturn ['db' => ['host' => 'lerd-mysql']];\n")
	if got := Reader(arr, "php-array")("db.host"); got != "lerd-mysql" {
		t.Errorf("php-array reader: %q", got)
	}
	konst := writeTemp(t, "<?php\ndefine('DB_HOST', 'lerd-mysql');\n")
	if got := Reader(konst, "php-const")("DB_HOST"); got != "lerd-mysql" {
		t.Errorf("php-const reader: %q", got)
	}
	dot := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(dot, []byte("DB_HOST=lerd-mysql\n"), 0o644)
	if got := Reader(dot, "dotenv")("DB_HOST"); got != "lerd-mysql" {
		t.Errorf("dotenv reader: %q", got)
	}
	// A missing file must not panic or error out the caller.
	if got := Reader(filepath.Join(t.TempDir(), "nope.php"), "php-array")("x"); got != "" {
		t.Errorf("missing file: %q", got)
	}
}

func TestApplyPhpArrayUpdates_NoOpWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.php")

	if err := ApplyPhpArrayUpdates(path, map[string]string{"db.host": "lerd-mysql"}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)

	// Re-applying the values the file already holds must not touch it: the writer
	// reprints the whole file, so a rewrite would churn a Magento deployment config
	// on every worktree sync.
	time.Sleep(10 * time.Millisecond)
	if err := ApplyPhpArrayUpdates(path, map[string]string{"db.host": "lerd-mysql"}); err != nil {
		t.Fatalf("second write: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("unchanged content rewrote the file: mtime %v -> %v", before.ModTime(), after.ModTime())
	}
	if now, _ := os.ReadFile(path); string(now) != string(body) {
		t.Errorf("content changed on a no-op write")
	}

	// A real change still lands.
	if err := ApplyPhpArrayUpdates(path, map[string]string{"db.host": "lerd-mariadb-11-8"}); err != nil {
		t.Fatalf("third write: %v", err)
	}
	got, _ := ReadPhpArray(path)
	if got["db.host"] != "lerd-mariadb-11-8" {
		t.Errorf("db.host = %q, want lerd-mariadb-11-8", got["db.host"])
	}
}
