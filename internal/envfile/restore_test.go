package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreValues(t *testing.T) {
	const phpHead = "<?php\n"
	cases := []struct{ name, format, before, now, want string }{
		{"old values back, new keys and layout kept", "dotenv",
			"APP_NAME=\"My App\"\nDB_HOST=127.0.0.1\nDB_PASSWORD=\n",
			"# mine\nAPP_NAME=Lerd\nDB_HOST=lerd-mysql\nDB_PASSWORD=lerd\nSTRIPE_KEY=abc\n",
			"# mine\nAPP_NAME=\"My App\"\nDB_HOST=127.0.0.1\nDB_PASSWORD=\nSTRIPE_KEY=abc\n"},
		{"a key lerd dropped comes back", "dotenv",
			"DB_HOST=127.0.0.1\n", "NEW=1\n", "NEW=1\nDB_HOST=127.0.0.1\n"},
		{"same values in other quotes stay as they are", "dotenv",
			"A=\"x\"\n", "A=x\n", "A=x\n"},
		{"a multi-line value comes back whole", "dotenv",
			"KEY=\"one\ntwo\"\nB=1\n", "A=1\nKEY=x\nB=2\n", "A=1\nKEY=\"one\ntwo\"\nB=1\n"},
		{"a missing multi-line value comes back whole", "dotenv",
			"KEY=\"one\ntwo\"\n", "A=1\n", "A=1\nKEY=\"one\ntwo\"\n"},
		{"a multi-line value is replaced whole", "dotenv",
			"KEY=old\n", "KEY=\"one\ntwo\"\nNEW=\"a\nb\"\n", "KEY=old\nNEW=\"a\nb\"\n"},
		{"repeated keys stay in step", "dotenv",
			"A=1\nA=2\n", "A=9\nA=9\n", "A=1\nA=2\n"},
		{"export and an apostrophe in a plain value", "dotenv",
			"export A=it's\nB=1\n", "export A=x\nB=2\n", "export A=it's\nB=1\n"},
		{"php constants", "php-const",
			phpHead + "define('DB_HOST', '127.0.0.1');\n",
			phpHead + "define('DB_HOST', 'lerd-mysql');\ndefine('MINE', 'yes');\n",
			phpHead + "define('DB_HOST', '127.0.0.1');\ndefine('MINE', 'yes');\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path, backup := filepath.Join(dir, "env"), filepath.Join(dir, "env.before_lerd")
			os.WriteFile(path, []byte(tc.now), 0644)
			os.WriteFile(backup, []byte(tc.before), 0644)
			if err := RestoreValues(path, backup, tc.format); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
			if changed, err := ChangedSince(path, backup, tc.format); changed || err != nil {
				t.Errorf("still changed after a restore (%v)", err)
			}
		})
	}
}

// A value whose type the env file still has comes back as that type.
func TestRestoreValues_keepsPHPTypes(t *testing.T) {
	dir := t.TempDir()
	path, backup := filepath.Join(dir, "a.php"), filepath.Join(dir, "a.php.before_lerd")
	os.WriteFile(backup, []byte("<?php\nreturn ['port' => 3306, 'tls' => false, 'socket' => null];\n"), 0644)
	os.WriteFile(path, []byte("<?php\nreturn ['port' => 3307, 'tls' => true, 'socket' => null];\n"), 0644)
	if err := RestoreValues(path, backup, "php-array"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if want := "<?php\nreturn ['port' => 3306, 'tls' => false, 'socket' => null];\n"; string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Nested arrays keep keys added since, at every level.
func TestRestoreValues_phpArray(t *testing.T) {
	dir := t.TempDir()
	path, backup := filepath.Join(dir, "a.php"), filepath.Join(dir, "a.php.before_lerd")
	os.WriteFile(backup, []byte("<?php\nreturn ['db' => ['host' => 'db.example']];\n"), 0644)
	os.WriteFile(path, []byte("<?php\nreturn ['db' => ['host' => 'lerd-mysql', 'extra' => 'kept']];\n"), 0644)
	if err := RestoreValues(path, backup, "php-array"); err != nil {
		t.Fatal(err)
	}
	got := Values(path, "php-array")
	if got["db.host"] != "db.example" || got["db.extra"] != "kept" {
		t.Errorf("values = %v", got)
	}
}

// A value only PHP can work out cannot be merged, so the file is left alone.
func TestRestoreValues_refusesWhatItCannotPutBackWhole(t *testing.T) {
	cases := []struct{ name, format, before, now string }{
		{"array expression replaced", "php-array",
			"<?php\nreturn ['Datasources' => env('DATABASE_URL')];\n",
			"<?php\nreturn ['Datasources' => ['default' => ['host' => 'lerd-mysql']]];\n"},
		{"vars expression replaced", "php-vars",
			"<?php\n$databases['default'] = getenv('DB');\n",
			"<?php\n$databases['default'] = ['host' => 'lerd-mysql'];\n"},
		{"constant expression replaced", "php-const",
			"<?php\ndefine('DB_HOST', getenv('DB_HOST'));\n",
			"<?php\ndefine('DB_HOST', 'lerd-mysql');\n"},
		{"null would come back as an empty string", "php-array",
			"<?php\nreturn ['password' => null];\n", "<?php\nreturn ['password' => 'lerd'];\n"},
		{"null became an empty string", "php-array",
			"<?php\nreturn ['password' => null];\n", "<?php\nreturn ['password' => ''];\n"},
		{"false would come back as a truthy string", "php-vars",
			"<?php\n$db['tls'] = false;\n", "<?php\n$db['tls'] = 'lerd';\n"},
		{"a number would come back as a string", "php-array",
			"<?php\nreturn ['port' => 3306];\n", "<?php\nreturn ['port' => 'lerd'];\n"},
		{"a key added under an old scalar would be lost", "php-array",
			"<?php\nreturn ['db' => 'x'];\n",
			"<?php\nreturn ['db' => ['host' => 'lerd-mysql', 'mine' => 'y']];\n"},
		{"unreadable backup", "php-array", "<?php\nreturn [;\n", "<?php\nreturn ['a' => 'b'];\n"},
		{"unknown format", "toml", "a=1\n", "a=2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path, backup := filepath.Join(dir, "f.php"), filepath.Join(dir, "f.php.before_lerd")
			os.WriteFile(path, []byte(tc.now), 0644)
			os.WriteFile(backup, []byte(tc.before), 0644)
			if err := RestoreValues(path, backup, tc.format); err == nil {
				t.Error("want an error")
			}
			if got, _ := os.ReadFile(path); string(got) != tc.now {
				t.Errorf("file changed to %q", got)
			}
			if changed, err := ChangedSince(path, backup, tc.format); !changed && err == nil {
				t.Error("the copy must still count as different")
			}
		})
	}
}

// An expression both files share is no reason to refuse.
func TestRestoreValues_sharedExpressionIsKept(t *testing.T) {
	dir := t.TempDir()
	path, backup := filepath.Join(dir, "f.php"), filepath.Join(dir, "f.php.before_lerd")
	os.WriteFile(backup, []byte("<?php\nreturn ['debug' => env('D'), 'host' => 'old'];\n"), 0644)
	os.WriteFile(path, []byte("<?php\nreturn ['debug' => env('D'), 'host' => 'lerd-mysql'];\n"), 0644)
	if err := RestoreValues(path, backup, "php-array"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "env('D')") || !strings.Contains(string(got), "'old'") {
		t.Errorf("got %q", got)
	}
}
