package serviceops

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// A service woken from idle-suspend starts from its unchanged unit, its
// dependency first, and never rewrites the quadlet (whose refresh can ask a
// registry for a newer tag while a request waits).
func TestWakeService_startsInstalledUnitsDependencyFirstWithoutRewriting(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	for _, n := range []string{"mysql", "phpmyadmin"} {
		if err := config.SaveCustomService(&config.CustomService{Name: n, Image: "docker.io/library/alpine:latest"}); err != nil {
			t.Fatal(err)
		}
	}
	pma, _ := config.LoadCustomService("phpmyadmin")
	pma.DependsOn = []string{"mysql"}
	if err := config.SaveCustomService(pma); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.QuadletDir(), 0755); err != nil {
		t.Fatal(err)
	}
	var quadlets []string
	for _, n := range []string{"mysql", "phpmyadmin"} {
		p := filepath.Join(config.QuadletDir(), "lerd-"+n+".container")
		if err := os.WriteFile(p, []byte("[Container]\nImage=as-installed\n"), 0644); err != nil {
			t.Fatal(err)
		}
		quadlets = append(quadlets, p)
	}

	var order []string
	prevStart, prevWait := wakeStartUnit, waitReadyFn
	t.Cleanup(func() { wakeStartUnit, waitReadyFn = prevStart, prevWait })
	wakeStartUnit = func(unit string) error { order = append(order, "start "+unit); return nil }
	waitReadyFn = func(name string, _ time.Duration) error { order = append(order, "ready "+name); return nil }

	if err := WakeService("phpmyadmin"); err != nil {
		t.Fatal(err)
	}
	want := []string{"start lerd-mysql", "ready mysql", "start lerd-phpmyadmin", "ready phpmyadmin"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for _, p := range quadlets {
		if data, _ := os.ReadFile(p); string(data) != "[Container]\nImage=as-installed\n" {
			t.Fatalf("%s was rewritten:\n%s", p, data)
		}
	}
}

// Opening an admin tool wakes the engines it administers that idle-suspend
// put to sleep, and leaves one the user stopped alone.
func TestWakeService_wakesTheSleepingEnginesAnAdminToolAdministers(t *testing.T) {
	withServiceHome(t)
	stubLifecycle(t)
	if err := os.MkdirAll(config.QuadletDir(), 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"mysql-8", "pg-16", "adminer"} {
		svc := &config.CustomService{Name: n, Image: "docker.io/library/alpine:latest"}
		switch n {
		case "mysql-8":
			svc.Family = "mysql"
		case "pg-16":
			svc.Family = "postgres"
		case "adminer":
			svc.AdminFor = []string{"mysql", "postgres"}
		}
		if err := config.SaveCustomService(svc); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(config.QuadletDir(), "lerd-"+n+".container"), []byte("[Container]\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	_ = config.SetServiceIdleSuspended("mysql-8", true) // asleep: wakes
	// pg-16 is simply stopped: stays down

	var started []string
	prevStart, prevWait := wakeStartUnit, waitReadyFn
	t.Cleanup(func() { wakeStartUnit, waitReadyFn = prevStart, prevWait })
	wakeStartUnit = func(unit string) error { started = append(started, unit); return nil }
	waitReadyFn = func(string, time.Duration) error { return nil }

	if err := WakeService("adminer"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started, []string{"lerd-mysql-8", "lerd-adminer"}) {
		t.Fatalf("started %v, want the sleeping engine then the tool", started)
	}
	if got := AdminToolsFor("pg-16"); !reflect.DeepEqual(got, []string{"adminer"}) {
		t.Fatalf("AdminToolsFor(pg-16) = %v", got)
	}
}
