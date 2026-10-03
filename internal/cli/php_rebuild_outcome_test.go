package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRebuildOutcomeRestartsEveryVersionWhenAllBuilt(t *testing.T) {
	restart, err := rebuildOutcome([]string{"8.4", "8.5"}, nil)
	if err != nil || !reflect.DeepEqual(restart, []string{"8.4", "8.5"}) {
		t.Errorf("restart=%v err=%v, want both and no error", restart, err)
	}
}

// A failed build leaves no image to restart onto, so its container keeps the
// old one and the command fails instead of reporting the image rebuilt.
func TestRebuildOutcomeSkipsAndReportsAFailedVersion(t *testing.T) {
	failed := map[string]error{"8.5": errors.New("exit status 125")}
	restart, err := rebuildOutcome([]string{"8.4", "8.5"}, failed)
	if !reflect.DeepEqual(restart, []string{"8.4"}) {
		t.Errorf("restart=%v, want only 8.4", restart)
	}
	if err == nil || !strings.Contains(err.Error(), "PHP 8.5") {
		t.Errorf("err=%v, want it to name PHP 8.5", err)
	}
}
