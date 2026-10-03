package tui

import (
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A sleeping service's dashboard is a bare localhost port nothing else wakes, so
// the key must start the service first, and a failed start opens no browser.
func TestOpenServiceDashboard_wakesASleepingServiceFirst(t *testing.T) {
	if browserOpener() == "" {
		t.Skip("no browser opener on " + runtime.GOOS)
	}
	prev := tuiWakeService
	t.Cleanup(func() { tuiWakeService = prev })
	var woke []string
	tuiWakeService = func(name string) error {
		woke = append(woke, name)
		return errors.New("image gone")
	}

	m := NewModel("test")
	m.snap = Snapshot{Services: []ServiceRow{{Name: "mailpit", Dashboard: "http://localhost:8025", State: stateSuspended}}}
	m.activeTab = tabServices

	cmd := m.openServiceDashboardCmd()
	if cmd == nil {
		t.Fatal("no command for a sleeping service with a dashboard")
	}
	res, ok := cmd().(ActionResult)
	if !ok || res.Err == nil || res.Summary != "wake mailpit" {
		t.Fatalf("got %#v, want the wake failure and no open", res)
	}
	if len(woke) != 1 || woke[0] != "mailpit" {
		t.Fatalf("woke %v", woke)
	}
}

// Opening the Databases tab wakes the sleeping engines, and only those: a
// sleeping redis holds no databases and stays asleep.
func TestWakeSleepingEngines_wakesOnlyDatabaseEngines(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, n := range []string{"mysql", "redis"} {
		_ = config.SetServiceIdleSuspended(n, true)
	}
	prev := tuiWakeService
	t.Cleanup(func() { tuiWakeService = prev })
	var mu sync.Mutex
	var woke []string
	tuiWakeService = func(n string) error {
		mu.Lock()
		woke = append(woke, n)
		mu.Unlock()
		return nil
	}

	wakeSleepingEngines()
	if len(woke) != 1 || woke[0] != "mysql" {
		t.Fatalf("woke %v, want only mysql", woke)
	}
}
