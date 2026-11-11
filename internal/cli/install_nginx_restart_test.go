package cli

import "testing"

func TestNginxNeedsRestart(t *testing.T) {
	cases := []struct {
		name                       string
		running, quadletChanged    bool
		containerImage, localImage string
		want                       bool
	}{
		{"running on the local image reloads", true, false, "aaa", "aaa", false},
		{"stopped container restarts", false, false, "aaa", "aaa", true},
		{"quadlet rewritten restarts", true, true, "aaa", "aaa", true},
		{"container on an older image than the local tag restarts", true, false, "aaa", "bbb", true},
		{"local image unknown restarts", true, false, "aaa", "", true},
		{"container image unknown restarts", true, false, "", "aaa", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nginxNeedsRestart(c.running, c.quadletChanged, c.containerImage, c.localImage); got != c.want {
				t.Errorf("nginxNeedsRestart() = %v, want %v", got, c.want)
			}
		})
	}
}
