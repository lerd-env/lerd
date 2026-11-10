package cli

import (
	"io"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// globalMCPOffer holds what offerGlobalMCP needs from the machine, so the
// decision can be tested without a terminal or an AI client installed.
type globalMCPOffer struct {
	enabled  func() bool
	declined func() bool
	decline  func()
	ask      func(question string) (yes, answered bool)
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
		ask:     askGlobalMCP,
		enable:  RunMCPEnableGlobal,
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
	if unattended || o.declined() || o.enabled() {
		return
	}
	yes, answered := o.ask("Let your AI assistants (Claude Code, Cursor, Codex…) use lerd through MCP (runs lerd mcp:enable-global)?")
	if !answered {
		return
	}
	if !yes {
		o.decline()
		feedback.Note("not asking again, turn it on any time under System > Lerd > AI assistants (MCP) in the dashboard, or with lerd mcp:enable-global")
		return
	}
	if err := o.enable(); err != nil {
		feedback.Warn("could not enable lerd MCP globally: %v", err)
	}
}

// readGlobalMCPAnswer counts only a finished line as an answer, so an EOF
// leaves MCP off without saving a no.
func readGlobalMCPAnswer(r io.Reader) (yes, answered bool) {
	buf := make([]byte, 1)
	var line strings.Builder
	for {
		n, err := r.Read(buf)
		if n > 0 && buf[0] == '\n' {
			a := strings.ToLower(strings.TrimSpace(line.String()))
			return a != "n" && a != "no", true
		}
		if n > 0 {
			line.WriteByte(buf[0])
		}
		if err != nil {
			return false, false
		}
	}
}
