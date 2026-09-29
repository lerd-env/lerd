package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/siteinfo"
)

func settingsRowsWith(t *testing.T, cfg *config.GlobalConfig) []settingsRow {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	return NewModel("test").settingsRows()
}

func TestSettingsOfferTheWebUIToggles(t *testing.T) {
	cfg := &config.GlobalConfig{}
	cfg.IdleSuspend.Enabled = true
	rows := settingsRowsWith(t, cfg)
	var labels []string
	on := map[settingsKind]bool{}
	for _, r := range rows {
		labels = append(labels, r.label)
		on[r.kind] = r.on
	}
	all := strings.Join(labels, "\n")
	for _, want := range []string{"Idle suspend", "Streaming mode", "Tray applet", "Notifications", "lerd DNS", "SPX profiler"} {
		if !strings.Contains(all, want) {
			t.Errorf("settings missing %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Xdebug") {
		t.Error("Xdebug moved to the PHP & Node view")
	}
	if !on[settingsIdle] || !on[settingsTray] || !on[settingsNotify] {
		t.Errorf("states not read from config: %+v", on)
	}
}

func TestSettingsToggleRunsTheMatchingVerb(t *testing.T) {
	rows := settingsRowsWith(t, &config.GlobalConfig{})
	m := NewModel("test")
	for i, r := range rows {
		if r.kind == settingsIdle {
			m.settingsRow = i
		}
	}
	if m.settingsToggle(rows) == nil || !strings.Contains(m.status, "idle suspend on") {
		t.Fatalf("toggling idle suspend should run lerd idle on, status %q", m.status)
	}
}

func TestSidebarSettingsEntryOpensAndLeavesSettings(t *testing.T) {
	m := NewModel("test")
	m.width, m.height = 160, 45
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{{Name: "shop", Domains: []string{"shop.test"}}}}
	m.sideFocus = true
	for _, it := range m.sideItems() {
		if it.key == "settings" {
			m.sideSelect(it)
		}
	}
	if m.detailMode != detailSettings || m.sideKeyFromState() != "settings" {
		t.Fatalf("the Settings entry should open settings, mode %v key %q", m.detailMode, m.sideKeyFromState())
	}
	m.sideMove(1)
	if m.detailMode != detailSite {
		t.Fatal("picking a site from the sidebar should leave the settings window")
	}
}

func TestShortDuration(t *testing.T) {
	for in, want := range map[string]string{"30m": "30m", "2h": "2h", "1h30m": "1h30m", "45s": "45s"} {
		d, _ := time.ParseDuration(in)
		if got := shortDuration(d); got != want {
			t.Errorf("shortDuration(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestSettingsHintsOnlyListWhatApplies(t *testing.T) {
	m := NewModel("test")
	m.switchTab(tabSites)
	m.detailMode = detailSettings
	m.focusMain()
	hints := stripANSI(m.renderHints(200))
	if !strings.Contains(hints, "space toggle") || strings.Contains(hints, "s start") {
		t.Fatalf("settings hints should offer the toggle, not site actions: %s", hints)
	}
}
