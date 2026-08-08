package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Drupal's installer leaves settings.php read-only, which is correct for a
// deployed site and is not a reason for lerd to refuse to configure a local
// one. It failed with "permission denied" after already creating the databases,
// leaving the user to chmod by hand and undo the rest.
func TestApplyPhpVarsUpdates_writesAReadOnlyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.php")
	body := "<?php\n$databases['default']['default'] = ['driver' => 'sqlite'];\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}

	if err := ApplyPhpVarsUpdates(path, map[string]string{"databases.default.default.driver": "mysql"}); err != nil {
		t.Fatalf("ApplyPhpVarsUpdates on a read-only file: %v", err)
	}

	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "'mysql'") {
		t.Errorf("the file was not updated:\n%s", out)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Errorf("mode = %v, want the 444 the framework hardened it to", info.Mode().Perm())
	}
}

// Every format goes through the same writer, so a hardened dotenv or wp-config
// behaves the same way.
func TestWriters_allHandleAHardenedFile(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, seed string
		apply      func(string) error
		want       string
	}{
		{".env", "DB_HOST=old\n", func(p string) error {
			return ApplyUpdates(p, map[string]string{"DB_HOST": "lerd-mysql"})
		}, "lerd-mysql"},
		{"wp-config.php", "<?php\ndefine( 'DB_HOST', 'old' );\n", func(p string) error {
			return ApplyPhpConstUpdates(p, map[string]string{"DB_HOST": "lerd-mysql"})
		}, "lerd-mysql"},
		{"env.php", "<?php\nreturn ['db' => ['host' => 'old']];\n", func(p string) error {
			return ApplyPhpArrayUpdates(p, map[string]string{"db.host": "lerd-mysql"})
		}, "lerd-mysql"},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name)
		if err := os.WriteFile(path, []byte(c.seed), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatal(err)
		}
		if err := c.apply(path); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		out, _ := os.ReadFile(path)
		if !strings.Contains(string(out), c.want) {
			t.Errorf("%s not updated:\n%s", c.name, out)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o444 {
			t.Errorf("%s mode = %v, want 444 restored", c.name, info.Mode().Perm())
		}
	}
}

// A writable file keeps the mode it had, untouched.
func TestWriteFile_leavesAWritableFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyUpdates(path, map[string]string{"A": "2"}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 600 preserved", info.Mode().Perm())
	}
}
