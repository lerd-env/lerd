package cli

import (
	"path/filepath"
	"testing"
)

// lerd has to run composer with the home composer itself would pick, or a
// global auth.json in ~/.composer is never read and private packages fail.
func TestResolveComposerHome(t *testing.T) {
	const home = "/home/u"
	xdgDir := filepath.Join(home, ".config", "composer")
	legacy := filepath.Join(home, ".composer")

	cases := []struct {
		name   string
		env    map[string]string
		etcXDG bool
		exists []string
		want   string
	}{
		{"COMPOSER_HOME wins", map[string]string{"COMPOSER_HOME": "/opt/c"}, true, []string{xdgDir, legacy}, "/opt/c"},
		{"XDG in use and its dir exists", nil, true, []string{xdgDir, legacy}, xdgDir},
		{"XDG in use but only ~/.composer exists", nil, true, []string{legacy}, legacy},
		{"no XDG, the macOS case", nil, false, []string{xdgDir, legacy}, legacy},
		{"no XDG and nothing yet", nil, false, nil, legacy},
		{"XDG in use and nothing yet", nil, true, nil, xdgDir},
		{"an XDG_ variable counts as XDG", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, false, []string{xdgDir}, xdgDir},
		{"XDG_CONFIG_HOME moves the XDG dir", map[string]string{"XDG_CONFIG_HOME": "/cfg"}, false, []string{"/cfg/composer"}, "/cfg/composer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var environ []string
			for k, v := range tc.env {
				environ = append(environ, k+"="+v)
			}
			isDir := func(p string) bool {
				if p == "/etc/xdg" {
					return tc.etcXDG
				}
				for _, e := range tc.exists {
					if e == p {
						return true
					}
				}
				return false
			}
			if got := resolveComposerHome(environ, home, isDir); got != tc.want {
				t.Errorf("resolveComposerHome = %q, want %q", got, tc.want)
			}
		})
	}
}
