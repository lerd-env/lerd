package node

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	bin := filepath.Join(home, ".local", "bin", "mise")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\necho '[{\"version\":\"22.11.0\",\"installed\":true},{\"version\":\"18.1.0\",\"installed\":false}]'\n"
	if err := os.WriteFile(bin, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]bool{
		"22": false, "v22": false, "22.11": false, "22.11.0": false,
		"2": true, "22.1": true, "22.12.0": true, "18": true, "20": true,
		"": false, "default": false, "lts/iron": false, "20; rm": false,
	} {
		if got := Missing(v); got != want {
			t.Errorf("Missing(%q) = %v, want %v", v, got, want)
		}
	}
}
