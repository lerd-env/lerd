package podman

import (
	"path/filepath"
	"testing"
)

func TestUnderDir(t *testing.T) {
	for _, c := range []struct {
		path, dir string
		want      bool
	}{
		{"/home/me", "/home/me", true},
		{"/home/me/site", "/home/me", true},
		{"/home/me/site", "/home/me/", true},
		{"/home/mex/site", "/home/me", false},
		{"/srv/site", "/home/me", false},
	} {
		if got := underDir(c.path, c.dir); got != c.want {
			t.Errorf("underDir(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}

// On Windows a path uses the native backslash, so a home of C:\Users\me has to
// contain C:\Users\me\AppData\... for the container to be trusted to see it.
func TestUnderDirNativeSeparator(t *testing.T) {
	if filepath.Separator != '\\' {
		t.Skip("only a backslash platform spells paths this way")
	}
	for _, c := range []struct {
		path, dir string
		want      bool
	}{
		{`C:\Users\me\AppData\Local\lerd\bin\composer.phar`, `C:\Users\me`, true},
		{`C:\Users\me`, `C:\Users\me`, true},
		{`C:\Users\meX\x`, `C:\Users\me`, false},
	} {
		if got := underDir(c.path, c.dir); got != c.want {
			t.Errorf("underDir(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}
