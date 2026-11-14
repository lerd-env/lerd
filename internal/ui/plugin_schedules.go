package ui

import (
	"time"

	"github.com/geodro/lerd/internal/plugins"
)

// scheduleTick is how often lerd-ui looks for plugin schedules that are due;
// the shortest interval a schedule may ask for is a minute.
const scheduleTick = time.Minute

// watchPluginSchedules runs the plugins' allowed schedules as they fall due.
// lerd-ui is a single service per machine that outlives lerd stop, so each
// schedule has one runner, and RunDue itself pauses while lerd is stopped.
func watchPluginSchedules() {
	plugins.SetScheduleNotifier(broker.broadcastPlugins)
	go func() {
		for {
			time.Sleep(scheduleTick)
			plugins.RunDue(time.Now())
		}
	}()
}
