package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Regression for the host-worker consent bypass: the gate must authorize the
// resolved command that actually runs (the reload variant when a project opts
// in), not the plain w.Command, so approving the shown command can never let an
// unshown reload_command execute on the host. Runs non-interactive (test env has
// no tty), where an unapproved command is denied rather than prompted.
func TestHostWorkerConsent_KeyedOnResolvedCommand(t *testing.T) {
	site := siteWithReload(t, true, true) // reload on, chokidar present
	if err := config.SaveSites(&config.SiteRegistry{Sites: []config.Site{
		{Name: "app", Path: site},
	}}); err != nil {
		t.Fatalf("SaveSites: %v", err)
	}

	// Named "horizon" so siteWithReload's reload opt-in applies to it.
	worker := config.FrameworkWorker{
		Command:       "npm run dev",
		ReloadCommand: "touch /tmp/pwned",
		Host:          true,
		ProjectOrigin: true,
	}
	resolved := resolveWorkerCommand(site, "horizon", worker)
	if resolved == worker.Command {
		t.Fatalf("precondition: reload should resolve to the reload command, got %q", resolved)
	}

	// Approving only the shown plain command must NOT authorize the reload command.
	if err := config.ApproveSiteCommand("app", worker.Command); err != nil {
		t.Fatalf("ApproveSiteCommand: %v", err)
	}
	if err := approveHostCommand("app", resolved, "worker \"horizon\""); err == nil {
		t.Error("approving only the plain command must not authorize the reload command")
	}

	// Approving the resolved command authorizes exactly what runs.
	if err := config.ApproveSiteCommand("app", resolved); err != nil {
		t.Fatalf("ApproveSiteCommand(resolved): %v", err)
	}
	if err := approveHostCommand("app", resolved, "worker \"horizon\""); err != nil {
		t.Errorf("approving the resolved command should authorize it: %v", err)
	}
}
