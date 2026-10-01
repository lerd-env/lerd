package config

import (
	"path/filepath"
	"testing"
)

func TestWindowsBaseDirs(t *testing.T) {
	env := map[string]string{"APPDATA": `C:\Users\me\AppData\Roaming`, "LOCALAPPDATA": `C:\Users\me\AppData\Local`}
	get := func(k string) string { return env[k] }

	cases := map[string]string{
		"config": filepath.Join(env["APPDATA"]),
		"data":   filepath.Join(env["LOCALAPPDATA"]),
		"state":  filepath.Join(env["LOCALAPPDATA"], "state"),
		"cache":  filepath.Join(env["LOCALAPPDATA"], "cache"),
	}
	for kind, want := range cases {
		got, ok := windowsBase(kind, get)
		if !ok || got != want {
			t.Errorf("windowsBase(%q) = %q %v, want %q", kind, got, ok, want)
		}
	}
}

func TestWindowsBaseWithoutTheVariablesDefersToTheHomeLayout(t *testing.T) {
	if _, ok := windowsBase("data", func(string) string { return "" }); ok {
		t.Error("with LOCALAPPDATA unset there is no Windows base, so callers keep the home layout")
	}
	if _, ok := windowsBase("nonsense", func(string) string { return "x" }); ok {
		t.Error("an unknown kind has no base")
	}
}

func TestExeName(t *testing.T) {
	cases := []struct{ name, goos, want string }{
		{"mkcert", "windows", "mkcert.exe"},
		{"mkcert.exe", "windows", "mkcert.exe"},
		{"mkcert", "linux", "mkcert"},
		{"fnm", "darwin", "fnm"},
	}
	for _, c := range cases {
		if got := exeName(c.name, c.goos); got != c.want {
			t.Errorf("exeName(%q, %q) = %q, want %q", c.name, c.goos, got, c.want)
		}
	}
}
