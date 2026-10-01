//go:build windows

package services

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitVolume(t *testing.T) {
	cases := []struct {
		in             string
		src, dst, opts string
	}{
		{"/srv/site:/var/www", "/srv/site", "/var/www", ""},
		{"/srv/site:/var/www:z,ro", "/srv/site", "/var/www", "z,ro"},
		{"lerd-ssh-agent:/ssh-agent", "lerd-ssh-agent", "/ssh-agent", ""},
		{`C:\Users\me\site:/var/www`, `C:\Users\me\site`, "/var/www", ""},
		{`C:\Users\me\site:/var/www:Z`, `C:\Users\me\site`, "/var/www", "Z"},
		{"C:/Users/me/site:/var/www:ro", "C:/Users/me/site", "/var/www", "ro"},
	}
	for _, c := range cases {
		src, dst, opts, ok := splitVolume(c.in)
		if !ok || src != c.src || dst != c.dst || opts != c.opts {
			t.Errorf("splitVolume(%q) = %q %q %q %v, want %q %q %q", c.in, src, dst, opts, ok, c.src, c.dst, c.opts)
		}
	}
	if _, _, _, ok := splitVolume("nocolon"); ok {
		t.Error("a spec with no destination must not parse")
	}
}

func TestStripSELinuxVolOptsDriveLetter(t *testing.T) {
	got := stripSELinuxVolOpts(`C:\Users\me\site:/var/www:z`)
	if got != `C:\Users\me\site:/var/www` {
		t.Errorf("got %q", got)
	}
	if got := stripSELinuxVolOpts(`C:\a:/b:z,ro`); got != `C:\a:/b:ro` {
		t.Errorf("got %q", got)
	}
}

func TestIsAbsHostPath(t *testing.T) {
	for p, want := range map[string]bool{
		"/srv/x": true, `C:\x`: true, "C:/x": true, "lerd-vol": false, "rel/dir": false,
	} {
		if got := isAbsHostPath(p); got != want {
			t.Errorf("isAbsHostPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSplitVolumeDriveLetterOnBothSides(t *testing.T) {
	cases := []struct {
		in             string
		src, dst, opts string
	}{
		{`C:\Sites\app:C:\Sites\app:rw`, `C:\Sites\app`, `C:\Sites\app`, "rw"},
		{`C:\Sites\app:C:\Sites\app`, `C:\Sites\app`, `C:\Sites\app`, ""},
		{`/srv/x:D:/data:ro`, "/srv/x", "D:/data", "ro"},
	}
	for _, c := range cases {
		src, dst, opts, ok := splitVolume(c.in)
		if !ok || src != c.src || dst != c.dst || opts != c.opts {
			t.Errorf("splitVolume(%q) = %q %q %q %v", c.in, src, dst, opts, ok)
		}
	}
}

func TestMapContainerPathsTranslatesOnlyTheContainerSide(t *testing.T) {
	c := map[string][]string{
		"Volume": {
			`C:\Sites\app:C:\Sites\app:rw`,
			`C:\Users\me\.local\share\lerd\hosts:/etc/hosts:ro`,
			"lerd-ssh-agent:/ssh-agent",
		},
		"WorkingDir": {`C:\Sites\app`},
	}
	mapContainerPaths(c)
	want := []string{
		`C:\Sites\app:/mnt/c/Sites/app:rw`,
		`C:\Users\me\.local\share\lerd\hosts:/etc/hosts:ro`,
		"lerd-ssh-agent:/ssh-agent",
	}
	for i, w := range want {
		if c["Volume"][i] != w {
			t.Errorf("Volume[%d] = %q, want %q", i, c["Volume"][i], w)
		}
	}
	if c["WorkingDir"][0] != "/mnt/c/Sites/app" {
		t.Errorf("WorkingDir = %q", c["WorkingDir"][0])
	}
}

// A quadlet writes host paths as %h/..., expanded to the home directory only
// when the unit is translated. The container side has to be mapped after that
// expansion, or a destination of %h/x stays a Windows path once it is expanded.
func TestMapContainerPathsExpandsSpecifiersFirst(t *testing.T) {
	t.Setenv("HOME", `C:\Users\me`)
	t.Setenv("USERPROFILE", `C:\Users\me`)
	c := map[string][]string{
		"Volume":     {"%h/.local/share/lerd/run:%h/.local/share/lerd/run:rw", "%h:%h:rw"},
		"WorkingDir": {"%h/site"},
	}
	mapContainerPaths(c)
	if got, want := c["Volume"][0], `C:\Users\me/.local/share/lerd/run:/mnt/c/Users/me/.local/share/lerd/run:rw`; got != want {
		t.Errorf("Volume[0] = %q, want %q", got, want)
	}
	if got, want := c["Volume"][1], `C:\Users\me:/mnt/c/Users/me:rw`; got != want {
		t.Errorf("Volume[1] = %q, want %q", got, want)
	}
	if got, want := c["WorkingDir"][0], "/mnt/c/Users/me/site"; got != want {
		t.Errorf("WorkingDir = %q, want %q", got, want)
	}
}

// Embedded units name lerd's directories the Linux way, as %h/.local/share/lerd.
// A host that keeps them elsewhere has to have those references rebased, or
// every bind mount points at a path that does not exist.
func TestRebaseLerdDirsFollowsTheConfiguredDirs(t *testing.T) {
	data, cfgDir := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	in := "Volume=%h/.local/share/lerd/nginx/nginx.conf:/etc/nginx/nginx.conf:ro\n" +
		"Volume=%h/.config/lerd/x.yaml:/x:ro\nVolume=%h/site:/site\nExec=run %h/.local/share/lerd/a %h/.local/share/lerd/b\n"
	got := rebaseLerdDirs(in)

	for _, want := range []string{
		"Volume=" + filepath.Join(data, "lerd") + "/nginx/nginx.conf:/etc/nginx/nginx.conf:ro",
		"Volume=" + filepath.Join(cfgDir, "lerd") + "/x.yaml:/x:ro",
		"Volume=%h/site:/site", // anything else under %h is left for expansion
		"Exec=run " + filepath.Join(data, "lerd") + "/a " + filepath.Join(data, "lerd") + "/b",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
