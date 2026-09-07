package cli

import (
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
)

func TestRunDbSnapshotAutoToggle_persistsPolicy(t *testing.T) {
	withTempXDG(t)

	every, keepFor := "6h", "168h"
	keep := 10
	if err := runDbSnapshotAutoToggle(true, autoSnapshotOptions{every: &every, keep: &keep, keepFor: &keepFor}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.AutoSnapshotEnabled() || cfg.AutoSnapshotEvery() != 6*time.Hour ||
		cfg.AutoSnapshotKeep() != 10 || cfg.AutoSnapshotKeepFor() != 168*time.Hour {
		t.Fatalf("policy not persisted: %+v", cfg.AutoSnapshot)
	}

	// Disabling keeps the retention the user chose, so re-enabling doesn't
	// silently fall back to the defaults.
	if err := runDbSnapshotAutoToggle(false, autoSnapshotOptions{}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	cfg, _ = config.LoadGlobal()
	if cfg.AutoSnapshotEnabled() {
		t.Error("still enabled after off")
	}
	if cfg.AutoSnapshotKeep() != 10 || cfg.AutoSnapshotEvery() != 6*time.Hour {
		t.Errorf("disabling rewrote the retention: %+v", cfg.AutoSnapshot)
	}
}

func TestRunDbSnapshotAutoToggle_selection(t *testing.T) {
	withTempXDG(t)

	optIn := "opt-in"
	if err := runDbSnapshotAutoToggle(true, autoSnapshotOptions{selection: &optIn}); err != nil {
		t.Fatalf("enable opt-in: %v", err)
	}
	cfg, _ := config.LoadGlobal()
	if cfg.AutoSnapshotSelection() != config.AutoSnapshotOptIn {
		t.Fatalf("selection = %q, want opt_in", cfg.AutoSnapshotSelection())
	}
	// A site that says nothing is not covered under opt-in.
	if cfg.AutoSnapshotCovers(&config.Site{Name: "shop"}) {
		t.Error("opt-in should cover no site until one opts in")
	}

	bad := "sometimes"
	if err := runDbSnapshotAutoToggle(true, autoSnapshotOptions{selection: &bad}); err == nil {
		t.Error("an unknown selection should be rejected")
	}
}

func TestRunDbSnapshotAutoToggle_rejectsBadDurations(t *testing.T) {
	withTempXDG(t)

	bad := "sometimes"
	if err := runDbSnapshotAutoToggle(true, autoSnapshotOptions{every: &bad}); err == nil {
		t.Error("an unparseable --every should be rejected")
	}
	if err := runDbSnapshotAutoToggle(true, autoSnapshotOptions{keepFor: &bad}); err == nil {
		t.Error("an unparseable --keep-for should be rejected")
	}
}

func TestRunDbSnapshotAutoSite(t *testing.T) {
	withTempXDG(t)
	site := config.Site{Name: "shop", Path: t.TempDir(), PHPVersion: "8.4"}
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{site}}); err != nil {
		t.Fatalf("save sites: %v", err)
	}

	if err := runDbSnapshotAutoSite("shop", "off"); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	reg, _ := config.LoadSites()
	if reg.Sites[0].AutoSnapshot != config.AutoSnapshotOff {
		t.Errorf("AutoSnapshot = %q, want off", reg.Sites[0].AutoSnapshot)
	}
	if err := runDbSnapshotAutoSite("shop", "nope"); err == nil {
		t.Error("an unknown mode should be rejected")
	}
}

func TestSnapshotRetentionCell(t *testing.T) {
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	manual := serviceops.Snapshot{Name: "pre-migration"}
	auto := serviceops.Snapshot{Name: "auto-1", Auto: true}

	tests := []struct {
		name string
		snap serviceops.Snapshot
		exp  serviceops.SnapshotExpiry
		want string
	}{
		{"manual", manual, serviceops.SnapshotExpiry{}, "manual"},
		{"kept", auto, serviceops.SnapshotExpiry{Kept: true}, "auto, kept"},
		{"estimated", auto, serviceops.SnapshotExpiry{At: now.Add(72 * time.Hour), Estimated: true}, "auto, ~3d"},
		{"exact age", auto, serviceops.SnapshotExpiry{At: now.Add(5 * time.Hour)}, "auto, 5h"},
		{"no limits", auto, serviceops.SnapshotExpiry{}, "auto"},
	}
	for _, tt := range tests {
		if got := snapshotRetentionCell(tt.snap, tt.exp, now); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestHumanEvery(t *testing.T) {
	tests := map[time.Duration]string{
		24 * time.Hour:     "24h",
		6 * time.Hour:      "6h",
		90 * time.Minute:   "1h30m",
		30 * time.Second:   "30s",
		7 * 24 * time.Hour: "168h",
	}
	for in, want := range tests {
		if got := humanEvery(in); got != want {
			t.Errorf("humanEvery(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestAutoSnapshotScheduleLine(t *testing.T) {
	cfg := &config.GlobalConfig{}
	cfg.AutoSnapshot.Every = "6h"
	cfg.AutoSnapshot.Keep = 3
	cfg.AutoSnapshot.KeepFor = "48h"
	cfg.AutoSnapshot.Selection = config.AutoSnapshotOptOut
	want := "every 6h, keeping 3 per database, dropping any older than 48h, covering every site that has not opted out"
	if got := autoSnapshotScheduleLine(cfg); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	cfg.AutoSnapshot.Keep = -1
	cfg.AutoSnapshot.KeepFor = ""
	if got := autoSnapshotScheduleLine(cfg); got != "every 6h, no count limit, covering every site that has not opted out" {
		t.Errorf("no-limit line = %q", got)
	}

	// Opt-in, the default, inverts the last clause: nothing is covered until a
	// site says yes.
	cfg.AutoSnapshot.Selection = config.AutoSnapshotOptIn
	if got := autoSnapshotScheduleLine(cfg); got != "every 6h, no count limit, covering only the sites that opted in" {
		t.Errorf("opt-in line = %q", got)
	}
}
