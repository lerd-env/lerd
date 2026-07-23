//go:build darwin

package dns

import (
	"strings"
	"testing"
)

// The passwordless probe must exercise a command the macOS drop-in actually
// grants, or it can never succeed: sudo refuses the ungranted command, the
// refusal reads as a conclusive "grant is gone", and lerd rewrites the sudoers
// rule (prompting for sudo) on every run. resolvectl is a Linux-only grant that
// never lands in the macOS drop-in, which is what broke issue #1101.
func TestSudoProbe_ExercisesACommandTheDarwinDropInGrants(t *testing.T) {
	grant := renderDarwinSudoers("kdonaldson", "test")

	probe := sudoersProbeCommand
	if len(sudoProbeArgs) > 0 {
		probe += " " + strings.Join(sudoProbeArgs, " ")
	}
	if !strings.Contains(grant, "NOPASSWD: "+probe) {
		t.Errorf("probe %q is not granted passwordless by the macOS sudoers drop-in:\n%s", probe, grant)
	}
}
