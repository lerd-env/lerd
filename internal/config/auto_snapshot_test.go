package config

import (
	"os"
	"testing"
	"time"
)

func TestAutoSnapshotDefaults(t *testing.T) {
	var nilCfg *GlobalConfig
	if !nilCfg.AutoSnapshotEnabled() {
		t.Error("nil config should report automatic snapshots on, matching defaultConfig")
	}
	cfg := &GlobalConfig{}
	// A directly built config carries an explicit false, the same way
	// AutoCleanupEnabled reads one; the shipped default comes from defaultConfig.
	if cfg.AutoSnapshotEnabled() {
		t.Error("an explicit false should report off")
	}
	if got := cfg.AutoSnapshotEvery(); got != DefaultAutoSnapshotEvery {
		t.Errorf("every = %v, want %v", got, DefaultAutoSnapshotEvery)
	}
	if got := cfg.AutoSnapshotKeep(); got != DefaultAutoSnapshotKeep {
		t.Errorf("keep = %d, want %d", got, DefaultAutoSnapshotKeep)
	}
	if got := cfg.AutoSnapshotKeepFor(); got != 0 {
		t.Errorf("keep_for = %v, want no age limit", got)
	}
}

func TestAutoSnapshotEvery_setAndBad(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.AutoSnapshot.Every = "6h"
	if got := cfg.AutoSnapshotEvery(); got != 6*time.Hour {
		t.Errorf("every = %v, want 6h", got)
	}
	for _, bad := range []string{"not-a-duration", "0s", "-1h"} {
		cfg.AutoSnapshot.Every = bad
		if got := cfg.AutoSnapshotEvery(); got != DefaultAutoSnapshotEvery {
			t.Errorf("every %q = %v, want the default", bad, got)
		}
	}
}

// A negative keep is how "no count limit" is expressed, so age retention can be
// the only rule that expires a snapshot.
func TestAutoSnapshotKeep_negativeMeansNoLimit(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.AutoSnapshot.Keep = 3
	if got := cfg.AutoSnapshotKeep(); got != 3 {
		t.Errorf("keep = %d, want 3", got)
	}
	cfg.AutoSnapshot.Keep = -1
	if got := cfg.AutoSnapshotKeep(); got != 0 {
		t.Errorf("keep = %d, want 0 (no count limit)", got)
	}
}

func TestAutoSnapshotKeepFor(t *testing.T) {
	cfg := &GlobalConfig{}
	cfg.AutoSnapshot.KeepFor = "168h"
	if got := cfg.AutoSnapshotKeepFor(); got != 168*time.Hour {
		t.Errorf("keep_for = %v, want 168h", got)
	}
	cfg.AutoSnapshot.KeepFor = "nonsense"
	if got := cfg.AutoSnapshotKeepFor(); got != 0 {
		t.Errorf("unparseable keep_for = %v, want no age limit", got)
	}
}

func TestLoadGlobal_AutoSnapshotFromFile(t *testing.T) {
	setConfigDir(t)
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "auto_snapshot:\n  enabled: true\n  every: 6h\n  keep: 10\n  keep_for: 72h\n"
	if err := os.WriteFile(GlobalConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidateGlobalCache()

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoSnapshotEnabled() {
		t.Error("auto_snapshot.enabled: true should turn the schedule on")
	}
	if got := cfg.AutoSnapshotSelection(); got != AutoSnapshotOptIn {
		t.Errorf("selection = %q, want the opt-in default when the file names none", got)
	}
	if got := cfg.AutoSnapshotEvery(); got != 6*time.Hour {
		t.Errorf("every = %v, want 6h", got)
	}
	if got := cfg.AutoSnapshotKeep(); got != 10 {
		t.Errorf("keep = %d, want 10", got)
	}
	if got := cfg.AutoSnapshotKeepFor(); got != 72*time.Hour {
		t.Errorf("keep_for = %v, want 72h", got)
	}
}

// The shipped default is on, and opt-in keeps it from dumping anything until a
// database is asked for; an explicit false in the file still turns it off.
func TestLoadGlobal_AutoSnapshotDefaultsOnAndOptIn(t *testing.T) {
	setConfigDir(t)
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoSnapshotEnabled() {
		t.Error("a fresh install should ship with the schedule on")
	}
	if got := cfg.AutoSnapshotSelection(); got != AutoSnapshotOptIn {
		t.Errorf("selection = %q, want opt-in", got)
	}
	if cfg.AutoSnapshotCovers(&Site{Name: "shop"}) {
		t.Error("a fresh install must cover no database until one is opted in")
	}
}

func TestLoadGlobal_AutoSnapshotHonoursExplicitDisable(t *testing.T) {
	setConfigDir(t)
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GlobalConfigFile(), []byte("auto_snapshot:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	invalidateGlobalCache()

	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoSnapshotEnabled() {
		t.Error("enabled: false should turn the schedule off")
	}
}

func TestNormalizeAutoSnapshotMode(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", AutoSnapshotDefault, false},
		{"default", AutoSnapshotDefault, false},
		{"on", AutoSnapshotOn, false},
		{" ON ", AutoSnapshotOn, false},
		{"off", AutoSnapshotOff, false},
		{"maybe", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeAutoSnapshotMode(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%q should be rejected", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%q = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAutoSnapshotSelection(t *testing.T) {
	var nilCfg *GlobalConfig
	if got := nilCfg.AutoSnapshotSelection(); got != AutoSnapshotOptIn {
		t.Errorf("nil config = %q, want opt-in", got)
	}
	cfg := &GlobalConfig{}
	if got := cfg.AutoSnapshotSelection(); got != AutoSnapshotOptIn {
		t.Errorf("unset = %q, want opt-in so a fresh install covers nothing unasked", got)
	}
	cfg.AutoSnapshot.Selection = AutoSnapshotOptOut
	if got := cfg.AutoSnapshotSelection(); got != AutoSnapshotOptOut {
		t.Errorf("opt-out = %q", got)
	}
	// A value nothing recognises is not a third mode.
	cfg.AutoSnapshot.Selection = "sometimes"
	if got := cfg.AutoSnapshotSelection(); got != AutoSnapshotOptIn {
		t.Errorf("unknown selection = %q, want opt-in", got)
	}
}

func TestNormalizeAutoSnapshotSelection(t *testing.T) {
	for _, in := range []string{"", "opt-in", "opt_in", " OPT-IN "} {
		got, err := NormalizeAutoSnapshotSelection(in)
		if err != nil || got != AutoSnapshotOptIn {
			t.Errorf("%q = %q (%v), want opt-in", in, got, err)
		}
	}
	for _, in := range []string{"opt-out", "opt_out"} {
		got, err := NormalizeAutoSnapshotSelection(in)
		if err != nil || got != AutoSnapshotOptOut {
			t.Errorf("%q = %q (%v), want opt-out", in, got, err)
		}
	}
	if _, err := NormalizeAutoSnapshotSelection("maybe"); err == nil {
		t.Error("an unknown selection mode should be rejected")
	}
}

// Under opt-in the schedule covers nothing until a site says yes; under opt-out
// it covers everything until a site says no. The override wins either way.
func TestAutoSnapshotCovers_selectionModes(t *testing.T) {
	optIn := &GlobalConfig{}
	optIn.AutoSnapshot.Enabled = true
	optIn.AutoSnapshot.Selection = AutoSnapshotOptIn

	tests := []struct {
		mode string
		want bool
	}{
		{AutoSnapshotDefault, false},
		{AutoSnapshotOn, true},
		{AutoSnapshotOff, false},
	}
	for _, tt := range tests {
		site := &Site{Name: "s", AutoSnapshot: tt.mode}
		if got := optIn.AutoSnapshotCovers(site); got != tt.want {
			t.Errorf("opt-in with mode %q: covers = %v, want %v", tt.mode, got, tt.want)
		}
	}

	optOut := &GlobalConfig{}
	optOut.AutoSnapshot.Enabled = true
	optOut.AutoSnapshot.Selection = AutoSnapshotOptOut
	if !optOut.AutoSnapshotCovers(&Site{Name: "s"}) {
		t.Error("opt-out should cover a site that says nothing")
	}
}

func TestAutoSnapshotCovers(t *testing.T) {
	on := &GlobalConfig{}
	on.AutoSnapshot.Enabled = true
	// Opt-out, so "follows the policy" means covered and the tri-state is what
	// the cases below are actually reading.
	on.AutoSnapshot.Selection = AutoSnapshotOptOut
	off := &GlobalConfig{}

	tests := []struct {
		name string
		cfg  *GlobalConfig
		mode string
		want bool
	}{
		{"global on, site follows", on, AutoSnapshotDefault, true},
		{"global on, site opted out", on, AutoSnapshotOff, false},
		// Off is off: the schedule's switch gates a site that opted in too.
		{"global off, site follows", off, AutoSnapshotDefault, false},
		{"global off, site opted in", off, AutoSnapshotOn, false},
		{"nil config, site opted in", nil, AutoSnapshotOn, true},
		{"nil config, site follows", nil, AutoSnapshotDefault, false},
	}
	for _, tt := range tests {
		site := &Site{Name: "s", AutoSnapshot: tt.mode}
		if got := tt.cfg.AutoSnapshotCovers(site); got != tt.want {
			t.Errorf("%s: covers = %v, want %v", tt.name, got, tt.want)
		}
	}
	if on.AutoSnapshotCovers(nil) {
		t.Error("a nil site is covered by nothing")
	}
}

func TestSetSiteAutoSnapshot(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	seed := Site{Name: "shop", Path: "/x", PHPVersion: "8.4", Pinned: true}
	if err := SaveSites(&SiteRegistry{Sites: []Site{seed}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := SetSiteAutoSnapshot("shop", AutoSnapshotOff); err != nil {
		t.Fatalf("SetSiteAutoSnapshot: %v", err)
	}
	reg, err := LoadSites()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := reg.Sites[0].AutoSnapshot; got != AutoSnapshotOff {
		t.Errorf("AutoSnapshot = %q, want %q", got, AutoSnapshotOff)
	}
	if !reg.Sites[0].Pinned {
		t.Error("the single-field write clobbered an unrelated field")
	}

	// Back to following the global policy, which stores the empty value.
	if err := SetSiteAutoSnapshot("shop", AutoSnapshotDefault); err != nil {
		t.Fatalf("SetSiteAutoSnapshot default: %v", err)
	}
	reg, _ = LoadSites()
	if got := reg.Sites[0].AutoSnapshot; got != AutoSnapshotDefault {
		t.Errorf("AutoSnapshot = %q, want the default", got)
	}

	if err := SetSiteAutoSnapshot("shop", "maybe"); err == nil {
		t.Error("an unknown mode should be rejected")
	}
	if err := SetSiteAutoSnapshot("missing", AutoSnapshotOn); err == nil {
		t.Error("an unknown site should be rejected")
	}
}
