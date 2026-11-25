package dns

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const herdResolver = "nameserver 127.0.0.1\n"

var lerdResolver = []byte("nameserver 127.0.0.1\nport 5300\n")

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func backupBody(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBackupForeignResolver(t *testing.T) {
	cases := []struct {
		name, current, backup string
		wantSaved             bool
		wantBackup            string
	}{
		{"another tool's file is saved", herdResolver, "", true, herdResolver},
		{"the tool's newer file replaces the old copy", "nameserver 127.0.0.1\nport 5353\n", herdResolver, true, "nameserver 127.0.0.1\nport 5353\n"},
		{"an unchanged copy is not reported again", herdResolver, herdResolver, false, herdResolver},
		{"lerd's own file never replaces the original", string(lerdResolver) + "\n", herdResolver, false, herdResolver},
		{"lerd's own file is not saved", string(lerdResolver), "", false, ""},
		{"a file its owner removed is not brought back", "", herdResolver, false, ""},
		{"nothing there", "", "", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "test")
			backup := filepath.Join(t.TempDir(), "backup", "test")
			if tc.current != "" {
				writeTestFile(t, file, tc.current)
			}
			if tc.backup != "" {
				writeTestFile(t, backup, tc.backup)
			}
			saved, err := backupForeignResolver(file, backup, lerdResolver)
			if err != nil {
				t.Fatal(err)
			}
			if saved != tc.wantSaved {
				t.Errorf("saved = %v, want %v", saved, tc.wantSaved)
			}
			if got := backupBody(t, backup); got != tc.wantBackup {
				t.Errorf("backup = %q, want %q", got, tc.wantBackup)
			}
		})
	}
}

// Overwriting a file that could not be saved would lose it.
func TestBackupForeignResolver_unwritableBackupIsAnError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test")
	writeTestFile(t, file, herdResolver)
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeTestFile(t, blocker, "a file where the directory should go")
	if _, err := backupForeignResolver(file, filepath.Join(blocker, "test"), lerdResolver); err == nil {
		t.Fatal("want an error when the copy cannot be written")
	}
}

func TestRestoreResolvers(t *testing.T) {
	dir := t.TempDir()
	withBackup, without := "/etc/resolver/test", "/etc/resolver/app"
	writeTestFile(t, filepath.Join(dir, "test"), herdResolver)
	// Not among lerd's files, so it must stay untouched.
	writeTestFile(t, filepath.Join(dir, "corp"), "nameserver 10.0.0.53\n")

	written := map[string]string{}
	left, err := restoreResolvers([]string{withBackup, without}, dir, func(p string, b []byte) error {
		written[p] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{withBackup: herdResolver}; !reflect.DeepEqual(written, want) {
		t.Errorf("written = %v, want %v", written, want)
	}
	if !reflect.DeepEqual(left, []string{without}) {
		t.Errorf("left to remove = %v, want %v", left, []string{without})
	}
	if got := backupBody(t, filepath.Join(dir, "test")); got != "" {
		t.Errorf("a used copy must go, still %q", got)
	}
}

// Removing lerd's file, or the copy, after a failed restore would lose the
// other tool's setup for good.
func TestRestoreResolvers_failureKeepsBoth(t *testing.T) {
	refuse := func(string, []byte) error { return os.ErrPermission }
	t.Run("write refused", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "test"), herdResolver)
		unsaved, err := restoreResolvers([]string{"/etc/resolver/test"}, dir, refuse)
		if err == nil || len(unsaved) != 0 {
			t.Errorf("unsaved = %v, err = %v", unsaved, err)
		}
		if got := backupBody(t, filepath.Join(dir, "test")); got != herdResolver {
			t.Errorf("copy = %q, want it kept", got)
		}
	})
	t.Run("copy unreadable", func(t *testing.T) {
		dir := t.TempDir()
		// A directory cannot be read as a file, also when the tests run as root.
		if err := os.Mkdir(filepath.Join(dir, "test"), 0755); err != nil {
			t.Fatal(err)
		}
		unsaved, err := restoreResolvers([]string{"/etc/resolver/test"}, dir, refuse)
		if err == nil || len(unsaved) != 0 {
			t.Errorf("unsaved = %v, err = %v", unsaved, err)
		}
	})
}
