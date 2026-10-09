//go:build darwin

package cli

import (
	"os"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/services"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
)

// installAutostart enables the lerd-autostart launchd service on macOS so that
// lerd starts automatically on every login. On macOS this is on by default
// (matching Herd's behaviour); on Linux it is opt-in via `lerd autostart enable`.
func installAutostart() {
	content, err := lerdSystemd.GetUnit("lerd-autostart")
	if err != nil {
		feedback.WarnOn(os.Stderr, "autostart unit: %v", err)
		return
	}
	if err := services.Mgr.WriteServiceUnit("lerd-autostart", content); err != nil {
		feedback.WarnOn(os.Stderr, "writing autostart service: %v", err)
		return
	}
	if err := services.Mgr.Enable("lerd-autostart"); err != nil {
		feedback.WarnOn(os.Stderr, "enabling autostart: %v", err)
	}
}

// syncLoginAutostart has nothing to do here: login start rides on the service
// units ApplyAutostart already enables or disables.
func syncLoginAutostart(bool) {}
