//go:build darwin

package ui

import (
	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/podman"
)

// init registers the worker-mode migration hooks the daemon needs to keep
// the podman API socket healthy through a mode toggle. The migration runs
// inside the lerd-ui process; without these hooks its sequential stop/rm/
// start podman calls would compete with this same process's open log
// streams (`podman logs -f`) and the container cache poller (`podman ps`)
// for gvproxy connection slots, eventually wedging the daemon.
func init() {
	cli.BeforeWorkerMigration = func(units []string) {
		logStreams.CancelAllFor(units)
		podman.Cache.Pause()
	}
	cli.AfterWorkerMigration = func() {
		podman.Cache.Resume()
	}
}
