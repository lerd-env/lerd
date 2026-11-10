package cli

import (
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// globalMCPOffer holds what offerGlobalMCP needs from the machine, so the
// decision can be tested without a terminal or an AI client installed.
type globalMCPOffer struct {
	enabled  func() bool
	declined func() bool
	decline  func()
	terminal func() bool
	ask      func(question string) bool
	enable   func() error
}

func defaultGlobalMCPOffer() globalMCPOffer {
	return globalMCPOffer{
		enabled: func() bool {
			home, err := os.UserHomeDir()
			return err == nil && mcpEnabledGlobally(home)
		},
		declined: func() bool {
			cfg, err := config.LoadGlobal()
			return err == nil && cfg.MCP.GlobalDeclined
		},
		decline: func() { saveGlobalMCPDeclined(true) },
		terminal: func() bool {
			_, closer, ok := promptSource()
			if closer != nil {
				closer.Close()
			}
			return ok
		},
		ask:    func(q string) bool { return confirmInstallPromptDefault(q, true) },
		enable: RunMCPEnableGlobal,
	}
}

// saveGlobalMCPDeclined persists the global MCP answer so install and update
// stop asking once someone has said no.
func saveGlobalMCPDeclined(declined bool) {
	cfg, err := config.LoadGlobal()
	if err != nil || cfg.MCP.GlobalDeclined == declined {
		return
	}
	cfg.MCP.GlobalDeclined = declined
	if err := config.SaveGlobal(cfg); err != nil {
		feedback.Warn("could not save the MCP choice: %v", err)
	}
}

// offerGlobalMCP asks on install and update whether to register lerd's MCP
// server with the AI assistants. Unattended or terminal-less runs never ask:
// writing into AI client configs needs a yes from a person.
func offerGlobalMCP(unattended bool, o globalMCPOffer) {
	if unattended || o.declined() || o.enabled() || !o.terminal() {
		return
	}
	if !o.ask("Let your AI assistants (Claude Code, Cursor, Codex…) use lerd through MCP (runs lerd mcp:enable-global)?") {
		o.decline()
		feedback.Note("not asking again, turn it on any time under System > Lerd > AI assistants (MCP) in the dashboard, or with lerd mcp:enable-global")
		return
	}
	if err := o.enable(); err != nil {
		feedback.Warn("could not enable lerd MCP globally: %v", err)
	}
}
