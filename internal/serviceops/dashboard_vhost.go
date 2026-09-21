package serviceops

import (
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nginx"
)

var syncLerdVhostFn = nginx.SyncLerdVhost

// syncDashboardVhost brings lerd's own vhost back in line with the services
// installed right now. The vhost names the path of every dashboard served at
// one, so a service installed after it was written is not among them and its
// console is closed by the catch-all until this runs. Called from the service
// operations that can change which dashboards exist or where they answer.
//
// Best effort on purpose: a dashboard that has to wait for the next `lerd
// start` is not worth failing an install, a removal or an update over, so a
// failure is warned about and the operation carries on.
func syncDashboardVhost() {
	changed, err := syncLerdVhostFn()
	if err != nil {
		feedback.Warn("regenerating lerd's vhost failed: %v; dashboards move on the next 'lerd start'", err)
		return
	}
	if !changed {
		return
	}
	if err := reloadNginxIfRunning(); err != nil {
		feedback.Warn("reloading nginx after the vhost changed failed: %v", err)
	}
}
