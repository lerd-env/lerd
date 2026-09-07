package nativephp

import (
	"os"
	"path/filepath"
	"testing"
)

// A running pool holds the config it started with, so a rewrite only reaches it
// if the unit is replaced. Knowing whether the file actually changed is what
// keeps that restart from happening on every single ensure.
func TestWriteIfChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "php-fpm.conf")

	changed, err := writeIfChanged(path, "pm = ondemand\n")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("writing a file that did not exist must report a change")
	}

	changed, err = writeIfChanged(path, "pm = ondemand\n")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("rewriting identical content must not report a change")
	}

	changed, err = writeIfChanged(path, "pm = dynamic\n")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("different content must report a change")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "pm = dynamic\n" {
		t.Errorf("file holds %q, want the new content", got)
	}
}
