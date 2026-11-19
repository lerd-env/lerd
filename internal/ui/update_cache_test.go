package ui

import "testing"

func TestUpdateCache_ComputesOncePerKey(t *testing.T) {
	c := newUpdateCache()
	calls := 0
	compute := func() bool { calls++; return true }
	cachedUpdate(c, "lerd-nginx", compute)
	cachedUpdate(c, "lerd-nginx", compute)
	if calls != 1 {
		t.Errorf("computed %d times across two status builds, want 1", calls)
	}
}

func TestUpdateCache_DropForUnitRecomputesOnlyThatUnit(t *testing.T) {
	c := newUpdateCache()
	nginx, redis := 0, 0
	cachedUpdate(c, "lerd-nginx", func() bool { nginx++; return false })
	cachedUpdate(c, "lerd-redis", func() bool { redis++; return false })

	c.dropFor("lerd-nginx")
	cachedUpdate(c, "lerd-nginx", func() bool { nginx++; return true })
	cachedUpdate(c, "lerd-redis", func() bool { redis++; return false })
	if nginx != 2 || redis != 1 {
		t.Errorf("nginx computed %d, redis %d, want 2 and 1", nginx, redis)
	}
}

// An empty name is a change lerd cannot pin to one unit, a tool update say,
// so everything is read again.
func TestUpdateCache_DropForEmptyRecomputesEverything(t *testing.T) {
	c := newUpdateCache()
	calls := 0
	compute := func() bool { calls++; return false }
	cachedUpdate(c, "lerd-nginx", compute)
	cachedUpdate(c, "tools", compute)
	c.dropFor("")
	cachedUpdate(c, "lerd-nginx", compute)
	cachedUpdate(c, "tools", compute)
	if calls != 4 {
		t.Errorf("computed %d times, want 4", calls)
	}
}
