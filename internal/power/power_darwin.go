//go:build darwin

package power

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// pmset runs a pmset query, bounded so a wedged pmset can never stall a worker
// write. A var so tests can drive the parsing without real hardware.
var pmset = func(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pmset", args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// detectState asks pmset for the power source and, separately, whether Low
// Power Mode is engaged. Low Power Mode can be on while plugged in, so it is
// checked first and wins.
func detectState() State {
	if lowPowerFromPmset(pmset("-g")) {
		return LowPower
	}
	if onBatteryFromPmset(pmset("-g", "batt")) {
		return Battery
	}
	return Mains
}

// onBatteryFromPmset reads pmset's "Now drawing from 'X Power'" line.
func onBatteryFromPmset(out string) bool {
	return strings.Contains(out, "'Battery Power'")
}

// lowPowerFromPmset reads Low Power Mode out of `pmset -g`, whose lines are a
// name and value separated by whitespace.
//
// The key was renamed across releases: older macOS reports "lowpowermode",
// current macOS reports "powermode". Both are accepted, since a machine only
// prints one of them and reading the wrong name means the mode silently never
// registers. "powermode" is tri-state (0 normal, 1 low, 2 high), so only an
// explicit 1 counts: high power mode must not be mistaken for low.
func lowPowerFromPmset(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "lowpowermode" || fields[0] == "powermode" {
			return fields[1] == "1"
		}
	}
	return false
}
