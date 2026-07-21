package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
	"github.com/geodro/lerd/internal/stats"
)

// TestDashboardGrid_RendersAllCards ensures every promised card title is
// present so the dashboard never silently loses a widget after a refactor.
func TestDashboardGrid_RendersAllCards(t *testing.T) {
	m := NewModel("test")
	joined := stripANSI(m.renderDashboardGrid(150, 30))
	for _, want := range []string{"Sites", "Services", "Workers", "System Health", "Resources", "Lerd"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing card %q in dashboard grid:\n%s", want, joined)
		}
	}
}

// TestWorkersCard_HealthyWhenNoFailures verifies the workers card reflects the
// heal state so users see the positive signal at a glance.
func TestWorkersCard_HealthyWhenNoFailures(t *testing.T) {
	m := NewModel("test")
	joined := stripANSI(strings.Join(m.dashWorkersCard(60, -1).lines, "\n"))
	if !strings.Contains(joined, "all workers healthy") {
		t.Errorf("expected healthy state with no failing workers:\n%s", joined)
	}
}

// TestWorkersCard_ShowsFailingCount counts failing workers from the snapshot
// so the card summary always matches the heal hint in the header.
func TestWorkersCard_ShowsFailingCount(t *testing.T) {
	m := NewModel("test")
	m.snap.Sites = []siteinfo.EnrichedSite{
		{Name: "a", QueueFailing: true, HasQueueWorker: true},
		{Name: "b", ScheduleFailing: true, HasScheduleWorker: true},
	}
	joined := stripANSI(strings.Join(m.dashWorkersCard(60, -1).lines, "\n"))
	if !strings.Contains(joined, "2 failing") {
		t.Errorf("expected '2 failing':\n%s", joined)
	}
	if !strings.Contains(joined, "press H") {
		t.Errorf("workers card should hint at H to heal:\n%s", joined)
	}
}

// TestResourcesCard_ShowsStatsWhenAvailable verifies the resources card
// renders concrete numbers from the cached snapshot, not the placeholder.
func TestResourcesCard_ShowsStatsWhenAvailable(t *testing.T) {
	m := NewModel("test")
	m.stats = stats.Snapshot{
		Available:       true,
		TotalCPUPercent: 12.5,
		TotalMemBytes:   128 * 1024 * 1024,
		HostMemBytes:    32 * 1024 * 1024 * 1024,
		Containers: []stats.ContainerStat{
			{Name: "lerd-mysql", CPUPercent: 5.5, MemBytes: 100 * 1024 * 1024},
			{Name: "lerd-redis", CPUPercent: 1.0, MemBytes: 28 * 1024 * 1024},
		},
	}
	joined := stripANSI(strings.Join(m.dashResourcesCard(60).lines, "\n"))
	if !strings.Contains(joined, "12.5%") {
		t.Errorf("expected '12.5%%' total CPU:\n%s", joined)
	}
	if !strings.Contains(joined, "lerd-mysql") {
		t.Errorf("expected top container 'lerd-mysql':\n%s", joined)
	}
	if strings.Contains(joined, "collecting") {
		t.Errorf("should not show placeholder when Available=true:\n%s", joined)
	}
}

// TestResourcesCard_PlaceholderWhenCollecting renders the polite "collecting…"
// message during the first window before the poller has run.
func TestResourcesCard_PlaceholderWhenCollecting(t *testing.T) {
	m := NewModel("test")
	// stats zero-valued: Available=false
	joined := stripANSI(strings.Join(m.dashResourcesCard(60).lines, "\n"))
	if !strings.Contains(joined, "collecting") {
		t.Errorf("expected 'collecting…' placeholder when stats unavailable:\n%s", joined)
	}
}

// stripANSI removes lipgloss escape sequences so tests can assert against
// the visible characters without coupling to the colour palette.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			// CSI introducer; params/intermediates are skipped until the
			// final byte (0x40–0x7E) ends the sequence. Handles both SGR
			// colour codes (…m) and bubblezone markers (…z).
			if r == '[' {
				continue
			}
			if r >= 0x40 && r <= 0x7e {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// The TUI shares lerd-ui's stats cache, so a poll at or above the TTL doesn't
// just cost the TUI a miss, it keeps the ~2s `podman stats` stream running for
// the web dashboard too.
func TestStatsPollStaysUnderCacheTTL(t *testing.T) {
	if statsPollInterval >= stats.CacheTTL {
		t.Fatalf("TUI polls every %v against a %v cache TTL, so every tick is a miss",
			statsPollInterval, stats.CacheTTL)
	}
}
