package cleanup

import (
	"reflect"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// A sleeping stripe listener has no container, so without this its image looks
// unused and cleanup offers to delete it.
func TestSleepingWorkerImagesFrom(t *testing.T) {
	cases := []struct {
		name  string
		sites []config.Site
		want  []string
	}{
		{"none asleep", []config.Site{{Name: "shop"}}, nil},
		{"queue asleep only", []config.Site{{Name: "shop", IdleSuspendedWorkers: []string{"queue"}}}, nil},
		{"stripe asleep", []config.Site{{Name: "shop", IdleSuspendedWorkers: []string{"queue", "stripe"}}}, []string{podman.StripeCLIImage}},
		{"stripe asleep in a worktree", []config.Site{{Name: "shop", WorktreeIdleSuspended: map[string][]string{"feat": {"stripe"}}}}, []string{podman.StripeCLIImage}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sleepingWorkerImagesFrom(tc.sites); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
