package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/stats"
)

// statsPollInterval is how often the background poller asks for stats. It has
// to stay below stats.CacheTTL: the cache is shared with lerd-ui, so a tick at
// or above the TTL would miss every time and keep the ~2s stream running.
const statsPollInterval = 3 * time.Second

// statsMsg delivers a fresh stats snapshot from the background goroutine to
// the bubbletea program. The poller is started by Run alongside the dumps
// listener so the dashboard pane always has data to render.
type statsMsg struct{ snap stats.Snapshot }

// runStatsPoller fetches a snapshot via stats.Cached on every tick and
// forwards it to the program. Going through Cached (rather than Read
// directly) means the TUI shares lerd-ui's cached snapshot when both are
// running, so the two surfaces together pay for one refresh per TTL.
// Cancelled by ctx so the loop exits cleanly on quit.
func runStatsPoller(ctx context.Context, p *tea.Program) {
	ticker := time.NewTicker(statsPollInterval)
	defer ticker.Stop()
	// First read happens immediately so the user doesn't see "no stats"
	// for a full tick after entering the dashboard.
	p.Send(statsMsg{snap: streamingStats()})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Send(statsMsg{snap: streamingStats()})
		}
	}
}

// streamingStats is the cached snapshot minus the containers of the sites
// streaming mode hides, which the resource card would otherwise name.
func streamingStats() stats.Snapshot {
	snap := stats.Cached(stats.CacheTTL)
	cfg, _ := config.LoadGlobal()
	reg, err := config.LoadSites()
	if err != nil {
		return snap
	}
	return stats.WithoutSites(snap, cfg.StreamingHidden(reg))
}
