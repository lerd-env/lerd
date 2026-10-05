package cli

import (
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// lerd install rewrites every vhost to the real one; a site whose database is
// still asleep then sent its requests to a stopped service and answered 500.
func TestRewakeSleepingSites_putsTheWakingVhostBack(t *testing.T) {
	f := installFakeIdleServices(t)
	f.users["mysql"] = []config.Site{{Name: "shop"}}
	if err := config.SetServiceIdleSuspended("mysql", true); err != nil {
		t.Fatal(err)
	}

	rewakeSleepingSites([]config.Site{{Name: "shop"}, {Name: "blog"}})

	if !slices.Equal(f.waking, []string{"shop"}) {
		t.Fatalf("waking vhosts = %v, want only shop, whose mysql sleeps", f.waking)
	}
	if f.reloads != 1 {
		t.Fatalf("nginx reloads = %d, want one after the swap", f.reloads)
	}
}

func TestRewakeSleepingSites_leavesAwakeSitesAlone(t *testing.T) {
	f := installFakeIdleServices(t)
	f.users["mysql"] = []config.Site{{Name: "shop"}}

	rewakeSleepingSites([]config.Site{{Name: "shop"}})

	if len(f.waking) != 0 || f.reloads != 0 {
		t.Fatalf("swapped %v with %d reloads, want nothing while mysql is awake", f.waking, f.reloads)
	}
}
