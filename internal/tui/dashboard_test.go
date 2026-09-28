package tui

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/siteinfo"
	"github.com/geodro/lerd/internal/stats"
)

// Every section is promised, so a refactor can never silently lose one.
func TestDashboard_RendersAllSections(t *testing.T) {
	m := NewModel("test")
	m.width, m.height = 150, 45
	m.snap.Sites = []siteinfo.EnrichedSite{{Name: "a", QueueFailing: true, HasQueueWorker: true}}
	joined := stripANSI(m.renderDashboard(140, 44))
	for _, want := range []string{"NEEDS ATTENTION", "RESOURCES", "SYSTEM", "RECENT"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing section %q:\n%s", want, joined)
		}
	}
}

func TestDashSystem_WorkersRowCountsCrashes(t *testing.T) {
	m := NewModel("test")
	joined := stripANSI(strings.Join(m.dashSystem(60), "\n"))
	if strings.Contains(joined, "crashed") {
		t.Errorf("no crashes should not mention crashed:\n%s", joined)
	}
	m.snap.Sites = []siteinfo.EnrichedSite{
		{Name: "a", QueueFailing: true, HasQueueWorker: true},
		{Name: "b", ScheduleFailing: true, HasScheduleWorker: true},
	}
	joined = stripANSI(strings.Join(m.dashSystem(60), "\n"))
	if !strings.Contains(joined, "2 crashed") {
		t.Errorf("expected '2 crashed':\n%s", joined)
	}
}

func TestDashResources_ShowsStatsWhenAvailable(t *testing.T) {
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
	joined := stripANSI(strings.Join(m.dashResources(70), "\n"))
	// Two decimals on both the total and the rows: CPU reads as a share of the
	// whole machine, so a single decimal rounds most rows away to 0.0.
	if !strings.Contains(joined, "12.50%") || !strings.Contains(joined, "5.50%") {
		t.Errorf("expected two-decimal CPU figures:\n%s", joined)
	}
	if !strings.Contains(joined, "mysql") || strings.Contains(joined, "collecting") {
		t.Errorf("expected the top container and no placeholder:\n%s", joined)
	}
}

func TestDashResources_PlaceholderWhenCollecting(t *testing.T) {
	m := NewModel("test")
	joined := stripANSI(strings.Join(m.dashResources(60), "\n"))
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
