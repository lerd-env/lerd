package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/eventbus"
)

// refreshMsg arrives on every tick. Update's handler reloads the snapshot
// off the main loop.
type refreshMsg struct{}

// busMsg signals that the eventbus subscription fired a publish. Distinct
// from refreshMsg so Update can re-chain busCmd to keep the subscription
// long-lived. tea.Cmd returns a single tea.Msg per invocation, so without
// the re-chain the channel stops being drained after the first publish.
type busMsg struct{}

// snapshotMsg carries a freshly-loaded Snapshot from a background goroutine
// back into the tea program.
type snapshotMsg struct{ snap Snapshot }

// tickCmd schedules the next refreshMsg. The TUI is push-driven via the
// podman cache OnChange callback (wired in Run) plus the eventbus
// subscription, so this passive tick is a safety net only — bumping the
// interval up to 10s avoids waking siteinfo.LoadAll and the snapshot
// rebuild every 2 seconds when nothing has changed.
func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return refreshMsg{} })
}

// loadCmd runs loadSnapshot off the main loop. siteinfo.LoadAll and podman
// calls can block for 100s of ms on slow systems; running them in the Update
// handler would freeze input.
func loadCmd() tea.Cmd {
	return func() tea.Msg { return snapshotMsg{snap: loadSnapshot()} }
}

// busCmd waits for the next publish on the eventbus subscription and
// emits a busMsg. The Update handler must re-chain busCmd on every busMsg
// or subsequent publishes pile up on the channel unread.
func busCmd(sub *eventbus.Subscriber) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-sub.C
		if !ok {
			return nil
		}
		return busMsg{}
	}
}
