//go:build !nogui

package tray

import (
	"strings"
	"testing"
)

func rowsFor(titles ...string) []row {
	out := make([]row, 0, len(titles))
	for _, t := range titles {
		out = append(out, row{title: t, args: []string{"use", t}})
	}
	return out
}

func TestFitRows_KeepsEverythingWhenItFits(t *testing.T) {
	in := rowsFor("a", "b", "c")
	got := fitRows(in, 5)
	if len(got) != 3 {
		t.Fatalf("fitRows kept %d rows, want 3", len(got))
	}
	for i := range in {
		if got[i].title != in[i].title {
			t.Errorf("row %d = %q, want %q", i, got[i].title, in[i].title)
		}
	}
}

func TestFitRows_FoldsOverflowIntoASummaryRow(t *testing.T) {
	got := fitRows(rowsFor("a", "b", "c", "d", "e"), 3)
	if len(got) != 3 {
		t.Fatalf("fitRows returned %d rows, want 3", len(got))
	}
	if got[0].title != "a" || got[1].title != "b" {
		t.Errorf("first rows = %q, %q; want a, b", got[0].title, got[1].title)
	}
	last := got[2]
	if !strings.Contains(last.title, "3 more") {
		t.Errorf("overflow row = %q, want it to report the 3 hidden entries", last.title)
	}
	if !last.disabled || len(last.args) != 0 {
		t.Errorf("overflow row must be inert, got disabled=%v args=%v", last.disabled, last.args)
	}
}

func TestFitRows_ZeroSizedPoolDropsEverything(t *testing.T) {
	if got := fitRows(rowsFor("a"), 0); got != nil {
		t.Errorf("fitRows with a zero pool = %v, want nil", got)
	}
}

func TestServiceRows_StatusDotAndClickAction(t *testing.T) {
	snap := &Snapshot{Services: []serviceInfo{
		{Name: "mysql", Status: "active"},
		{Name: "redis", Status: "inactive"},
		{Name: "meilisearch", Status: "inactive", Paused: true},
	}}

	got := serviceRows(snap)
	if len(got) != 3 {
		t.Fatalf("serviceRows returned %d rows, want 3", len(got))
	}

	cases := []struct {
		title string
		args  []string
	}{
		{"🟢 mysql", []string{"service", "stop", "mysql"}},
		{"🔴 redis", []string{"service", "start", "redis"}},
		{"🟡 meilisearch", []string{"service", "start", "meilisearch"}},
	}
	for i, want := range cases {
		if got[i].title != want.title {
			t.Errorf("row %d title = %q, want %q", i, got[i].title, want.title)
		}
		if strings.Join(got[i].args, " ") != strings.Join(want.args, " ") {
			t.Errorf("row %d args = %v, want %v", i, got[i].args, want.args)
		}
	}
}

func TestPHPRows_TicksTheDefaultVersion(t *testing.T) {
	snap := &Snapshot{
		PHPVersions: []phpInfo{{Version: "8.3"}, {Version: "8.4"}},
		PHPDefault:  "8.4",
	}

	got := phpRows(snap)
	if len(got) != 2 {
		t.Fatalf("phpRows returned %d rows, want 2", len(got))
	}
	if got[0].title != "8.3" {
		t.Errorf("non-default row = %q, want %q", got[0].title, "8.3")
	}
	if got[1].title != "✔ 8.4" {
		t.Errorf("default row = %q, want it ticked", got[1].title)
	}
	if strings.Join(got[1].args, " ") != "use 8.4" {
		t.Errorf("php row args = %v, want [use 8.4]", got[1].args)
	}
}

func TestServicesTitle(t *testing.T) {
	tests := []struct {
		name string
		snap *Snapshot
		want string
	}{
		{"empty", &Snapshot{}, "Services"},
		{"all running", &Snapshot{Services: []serviceInfo{
			{Name: "a", Status: "active"},
			{Name: "b", Status: "active"},
		}}, "Services (2/2)"},
		{"paused does not count as running", &Snapshot{Services: []serviceInfo{
			{Name: "a", Status: "active"},
			{Name: "b", Status: "active", Paused: true},
			{Name: "c", Status: "failed"},
		}}, "Services (1/3)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := servicesTitle(tc.snap); got != tc.want {
				t.Errorf("servicesTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWorkersTitle(t *testing.T) {
	tests := []struct {
		name     string
		snap     *Snapshot
		want     string
		wantShow bool
	}{
		{
			name: "nothing running and nothing broken stays hidden",
			snap: &Snapshot{},
		},
		{
			name: "idle-suspended sites do not raise an alarm",
			// 20 declared workers all stopped, health endpoint reports nothing:
			// exactly the shape of a machine with most sites idle-suspended.
			snap: &Snapshot{WorkersRunning: 0},
		},
		{
			name:     "single running worker is not pluralised",
			snap:     &Snapshot{WorkersRunning: 1},
			want:     "  🟢 1 worker",
			wantShow: true,
		},
		{
			name:     "running count",
			snap:     &Snapshot{WorkersRunning: 4},
			want:     "  🟢 4 workers",
			wantShow: true,
		},
		{
			name:     "a single failure names the site",
			snap:     &Snapshot{WorkersRunning: 3, WorkersDown: []string{"acme.test"}},
			want:     "  🔴 worker down (acme.test)",
			wantShow: true,
		},
		{
			name:     "several failures collapse to a count",
			snap:     &Snapshot{WorkersRunning: 3, WorkersDown: []string{"acme.test", "shop.test"}},
			want:     "  🔴 2 workers down",
			wantShow: true,
		},
		{
			name:     "failures win over the running count",
			snap:     &Snapshot{WorkersRunning: 9, WorkersDown: []string{"acme.test"}},
			want:     "  🔴 worker down (acme.test)",
			wantShow: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, show := workersTitle(tc.snap)
			if show != tc.wantShow {
				t.Fatalf("workersTitle() show = %v, want %v", show, tc.wantShow)
			}
			if got != tc.want {
				t.Errorf("workersTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestServiceInfo_IsWorker(t *testing.T) {
	if (serviceInfo{Name: "mysql", Status: "active"}).isWorker() {
		t.Error("a real service preset must not count as a worker")
	}
	workers := []serviceInfo{
		{Name: "queue-a", QueueSite: "a.test"},
		{Name: "schedule-a", ScheduleWorkerSite: "a.test"},
		{Name: "stripe-a", StripeListenerSite: "a.test"},
		{Name: "reverb-a", ReverbSite: "a.test"},
		{Name: "horizon-a", HorizonSite: "a.test"},
		{Name: "vite-a", WorkerSite: "a.test"},
	}
	for _, w := range workers {
		if !w.isWorker() {
			t.Errorf("%s should count as a per-site worker", w.Name)
		}
	}
}

func TestPHPTitle(t *testing.T) {
	if got := phpTitle(&Snapshot{}); got != "PHP" {
		t.Errorf("phpTitle with no default = %q, want %q", got, "PHP")
	}
	if got := phpTitle(&Snapshot{PHPDefault: "8.4"}); got != "PHP 8.4" {
		t.Errorf("phpTitle() = %q, want %q", got, "PHP 8.4")
	}
}

func TestToggleTitle(t *testing.T) {
	if got := toggleTitle("Notifications", true); got != "Notifications: ✔ On" {
		t.Errorf("toggleTitle(on) = %q", got)
	}
	if got := toggleTitle("Notifications", false); got != "Notifications: Off" {
		t.Errorf("toggleTitle(off) = %q", got)
	}
}

func TestManagedServiceLANTitleDistinguishesEffectiveState(t *testing.T) {
	cases := []struct {
		name string
		snap Snapshot
		want string
	}{
		{"off", Snapshot{}, "Managed service LAN access: Off"},
		{"armed", Snapshot{LANServicesExposed: true}, "Managed service LAN access: Armed (LAN exposure off)"},
		{"active", Snapshot{LANExposed: true, LANServicesExposed: true}, "Managed service LAN access: ✔ On"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := managedServiceLANTitle(&tc.snap); got != tc.want {
				t.Fatalf("managedServiceLANTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}
