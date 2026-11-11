package cli

import "testing"

func TestNginxNeedsRestart(t *testing.T) {
	same := []string{"sha256:aaa"}
	cases := []struct {
		name           string
		running        bool
		quadletChanged bool
		before, after  []string
		want           bool
	}{
		{"unchanged and running reloads", true, false, same, same, false},
		{"stopped container restarts", false, false, same, same, true},
		{"quadlet rewritten restarts", true, true, same, same, true},
		{"new image digest restarts", true, false, same, []string{"sha256:bbb"}, true},
		{"image missing before pull restarts", true, false, nil, same, true},
		{"pull failed with image kept reloads", true, false, same, same, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nginxNeedsRestart(c.running, c.quadletChanged, c.before, c.after); got != c.want {
				t.Errorf("nginxNeedsRestart() = %v, want %v", got, c.want)
			}
		})
	}
}
