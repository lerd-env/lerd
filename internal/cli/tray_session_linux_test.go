//go:build linux

package cli

import "testing"

// Over ssh with nobody logged in to a desktop, starting the tray only fails
// three times on "cannot open display" and leaves the user session degraded.
func TestHasGraphicalSession(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		manager string
		want    bool
	}{
		{"ssh with no desktop", nil, "HOME=/home/g\nPATH=/usr/bin\n", false},
		{"terminal in an X session", map[string]string{"DISPLAY": ":0"}, "", true},
		{"terminal in a Wayland session", map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, "", true},
		{"daemon, desktop imported into the manager", nil, "WAYLAND_DISPLAY=wayland-1\n", true},
		{"manager unreadable", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			if got := hasGraphicalSession(getenv, func() string { return tc.manager }); got != tc.want {
				t.Errorf("hasGraphicalSession = %v, want %v", got, tc.want)
			}
		})
	}
}
