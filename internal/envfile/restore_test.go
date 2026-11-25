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
			if left := ChangedSince(path, backup, tc.format); len(left) != 0 {
				t.Errorf("still changed after a restore: %v", left)
			}
		})
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

// The line writer cannot put half of a multi-line value back.
func TestRestoreValues_multiLineValueIsRefused(t *testing.T) {
	dir := t.TempDir()
	path, backup := filepath.Join(dir, "env"), filepath.Join(dir, "env.before_lerd")
	os.WriteFile(backup, []byte("KEY=\"one\ntwo\"\n"), 0644)
	os.WriteFile(path, []byte("A=1\n"), 0644)
	if err := RestoreValues(path, backup, "dotenv"); err == nil || !strings.Contains(err.Error(), "KEY") {
		t.Fatalf("err = %v, want one naming KEY", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "A=1\n" {
		t.Errorf("file changed to %q", got)
	}
}
