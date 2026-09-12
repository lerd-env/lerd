package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/services"
)

// daemonMgr answers whether the long-running lerd units are up.
type daemonMgr struct {
	services.ServiceManager
	active bool
}

func (m *daemonMgr) IsActive(string) bool { return m.active }

func useDaemonMgr(t *testing.T, active bool) {
	t.Helper()
	prev := services.Mgr
	services.Mgr = &daemonMgr{active: active}
	t.Cleanup(func() { services.Mgr = prev })
}

// Install owns the restart onto the binary it just laid down, including the
// autostart-off case `lerd update` used to cover with a second pass of its own.
func TestShouldRestartDaemon(t *testing.T) {
	cases := []struct {
		name        string
		autostartOn bool
		active      bool
		want        bool
	}{
		{"autostart on", true, false, true},
		{"autostart off, unit stopped", false, false, false},
		{"autostart off, unit running by hand", false, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			useDaemonMgr(t, c.active)
			if got := shouldRestartDaemon("lerd-ui", c.autostartOn); got != c.want {
				t.Errorf("shouldRestartDaemon = %v, want %v", got, c.want)
			}
		})
	}
}
