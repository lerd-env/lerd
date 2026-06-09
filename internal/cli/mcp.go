package cli

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/mcp"
	"github.com/spf13/cobra"
)

// NewMCPCmd returns the mcp command — starts the MCP server over stdio.
func NewMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the lerd MCP server (JSON-RPC 2.0 over stdio)",
		Long: `Starts a Model Context Protocol server that allows AI assistants
(Claude Code, Cursor, JetBrains Junie, etc.) to manage lerd sites, run artisan
commands, and control services.

This command is normally invoked automatically by the AI assistant via
the MCP configuration injected by 'lerd mcp:inject'.`,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return mcp.Serve()
		},
	}
}

// NewMCPInjectCmd returns the mcp:inject command.
func NewMCPInjectCmd() *cobra.Command {
	var targetPath string
	cmd := &cobra.Command{
		Use:   "mcp:inject",
		Short: "Inject lerd MCP config and AI skill files into a project",
		Long: `Writes MCP server config and context files for every supported AI
assistant into the target project directory:

  .mcp.json                        Claude Code MCP config
  .claude/skills/lerd/SKILL.md     Claude Code skill (lerd tools reference)
  .cursor/mcp.json                 Cursor MCP config
  .cursor/rules/lerd.mdc           Cursor rules file
  .junie/mcp/mcp.json              JetBrains Junie MCP config
  .junie/guidelines.md             JetBrains Junie guidelines
  .ai/mcp/mcp.json                 Windsurf MCP config
  .gemini/settings.json            Gemini CLI MCP config
  GEMINI.md                        Gemini CLI context
  .vscode/mcp.json                 GitHub Copilot (VS Code) MCP config
  .github/copilot-instructions.md  GitHub Copilot instructions
  AGENTS.md                        Codex CLI context (Codex MCP is global-only)

Run this from a Laravel project root, or use --path to specify a directory.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMCPInject(targetPath)
		},
	}
	cmd.Flags().StringVar(&targetPath, "path", "", "Target project directory (defaults to current directory)")
	return cmd
}

func runMCPInject(targetPath string) error {
	if targetPath == "" {
		var err error
		targetPath, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	fmt.Printf("Injecting lerd MCP config into: %s\n\n", abs)
	if err := WriteProjectAISkills(abs, true); err != nil {
		return err
	}
	fmt.Println("\nDone! Restart your AI assistant to load the lerd MCP server.")
	return nil
}

// WriteProjectAISkills writes the per-project AI artefacts for abs by iterating
// the aiClients registry, creating every client's files. MCP config files and
// sentinel context docs preserve non-lerd entries; overwrite docs (SKILL.md,
// lerd.mdc) are replaced only when their content changed. verbose=true prints
// each written path. This is the opt-in path (mcp:inject).
func WriteProjectAISkills(abs string, verbose bool) error {
	return writeProjectArtefacts(abs, verbose, true)
}

// RefreshProjectAISkills re-writes only the per-project artefacts a client was
// already set up with, so `lerd update` keeps existing files current without
// expanding a project's footprint with files for clients it never opted into.
func RefreshProjectAISkills(abs string, verbose bool) error {
	return writeProjectArtefacts(abs, verbose, false)
}

func writeProjectArtefacts(abs string, verbose, createMissing bool) error {
	log := func(msg string) {
		if verbose {
			fmt.Println(msg)
		}
	}

	for _, c := range aiClients {
		if c.ProjectMCP != "" {
			full := filepath.Join(abs, c.ProjectMCP)
			if createMissing || mcpConfigHasLerd(full, c) {
				if err := writeClientMCP(full, c, abs); err != nil {
					return err
				}
				log("  updated " + c.ProjectMCP)
			}
		}
		for _, cx := range c.Contexts {
			if cx.Project == "" {
				continue
			}
			full := filepath.Join(abs, cx.Project)
			if createMissing || contextAlreadyPresent(full, cx) {
				if err := writeClientContext(full, cx); err != nil {
					return fmt.Errorf("writing %s: %w", cx.Project, err)
				}
				log("  wrote   " + cx.Project)
			}
		}
	}

	return nil
}

// ProjectHasLerdSkills is the opt-in signal for project-scoped refresh: true
// iff at least one lerd-owned marker file exists. Shared JSON configs are not
// checked because they may contain unrelated MCP servers.
func ProjectHasLerdSkills(abs string) bool {
	if _, err := os.Stat(filepath.Join(abs, ".claude", "skills", "lerd", "SKILL.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(abs, ".cursor", "rules", "lerd.mdc")); err == nil {
		return true
	}
	if data, err := os.ReadFile(filepath.Join(abs, ".junie", "guidelines.md")); err == nil {
		if strings.Contains(string(data), "<!-- lerd:begin -->") {
			return true
		}
	}
	return false
}

// writeIfChanged only writes when content differs, so projects already current
// stay untouched (clean git status across upgrades).
func writeIfChanged(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if len(existing) == len(content) && string(existing) == string(content) {
			return nil
		}
	}
	return os.WriteFile(path, content, 0644)
}

// NewMCPEnableGlobalCmd returns the mcp:enable-global command.
func NewMCPEnableGlobalCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp:enable-global",
		Short: "Register lerd MCP globally for all AI assistant sessions",
		Long: `Registers the lerd MCP server at user scope so it is available
in every Claude Code session, regardless of the current project directory.

The server uses the directory Claude is opened in as the site context —
no LERD_SITE_PATH configuration needed.

This command updates:
  claude mcp add --scope user      Claude Code user-scope MCP registration
  ~/.cursor/mcp.json               Cursor global MCP config
  ~/.ai/mcp/mcp.json               Windsurf global MCP config
  ~/.junie/mcp/mcp.json            JetBrains Junie global MCP config
  ~/.gemini/settings.json          Gemini CLI global MCP config
  ~/.codex/config.toml             Codex CLI global MCP config
  ~/.config/Code/User/mcp.json     GitHub Copilot (VS Code) global MCP config
  ~/.gemini/config/mcp_config.json Google Antigravity global MCP config
  ~/.claude/skills/lerd/SKILL.md   Claude Code user-scope skill
  ~/.cursor/rules/lerd.mdc         Cursor user-scope rules
  ~/.junie/guidelines.md           JetBrains Junie user-scope guidelines
  ~/.gemini/GEMINI.md              Gemini CLI user-scope context
  ~/.codex/AGENTS.md               Codex CLI user-scope context`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return RunMCPEnableGlobal()
		},
	}
}

// RunMCPEnableGlobal registers lerd MCP at user scope for all supported AI tools.
// It is exported so the install command can call it directly.
func RunMCPEnableGlobal() error {
	fmt.Println("Registering lerd MCP globally...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if err := writeGlobalMCPConfigs(home, true); err != nil {
		return err
	}
	if err := WriteGlobalAISkills(home, true); err != nil {
		return err
	}

	fmt.Println("\nDone! Restart your AI assistant for changes to take effect.")
	fmt.Println("lerd will use the directory you open Claude in as the site context.")
	return nil
}

// writeGlobalMCPConfigs registers the user-scope MCP server for every client in
// the registry: file-backed clients via their global config, Claude via the
// idempotent `claude mcp add` CLI. Global entries omit LERD_SITE_PATH so the
// server uses the directory the assistant is opened in.
func writeGlobalMCPConfigs(home string, verbose bool) error {
	log := func(msg string) {
		if verbose {
			fmt.Println(msg)
		}
	}
	for _, c := range aiClients {
		if c.GlobalViaCLI {
			// Try remove first for idempotent re-registration, then add.
			_, _ = claudeMCP("remove", "--scope", "user", "lerd")
			out, err := claudeMCP("add", "--scope", "user", "lerd", "--", "lerd", "mcp")
			if err != nil {
				fmt.Printf("  warning: could not register with Claude Code (%v): %s\n", err, strings.TrimSpace(string(out)))
				fmt.Println("  Run manually: claude mcp add --scope user lerd -- lerd mcp")
			} else {
				log("  registered in Claude Code (user scope)")
			}
			continue
		}
		if c.GlobalMCP == "" {
			continue
		}
		if err := writeClientMCP(filepath.Join(home, c.GlobalMCP), c, ""); err != nil {
			return err
		}
		log("  updated ~/" + c.GlobalMCP)
	}
	return nil
}

// WriteGlobalAISkills writes the user-scope context/instructions docs for every
// client in the registry (SKILL.md, lerd.mdc, guidelines.md, GEMINI.md,
// AGENTS.md), creating any that are missing. Called from mcp:enable-global.
// MCP registration itself is handled separately by writeGlobalMCPConfigs.
func WriteGlobalAISkills(home string, verbose bool) error {
	return writeGlobalContexts(home, verbose, true)
}

// RefreshGlobalAISkills re-writes only the user-scope context docs that already
// exist, so `lerd update` keeps them aligned with the installed binary's tool
// set without creating files (~/.codex/AGENTS.md, ~/.gemini/GEMINI.md) for
// clients the user never enabled.
func RefreshGlobalAISkills(home string, verbose bool) error {
	return writeGlobalContexts(home, verbose, false)
}

func writeGlobalContexts(home string, verbose, createMissing bool) error {
	for _, c := range aiClients {
		for _, cx := range c.Contexts {
			if cx.Global == "" {
				continue
			}
			full := filepath.Join(home, cx.Global)
			if !createMissing && !contextAlreadyPresent(full, cx) {
				continue
			}
			if err := writeClientContext(full, cx); err != nil {
				return fmt.Errorf("writing %s: %w", cx.Global, err)
			}
			if verbose {
				fmt.Println("  wrote   ~/" + cx.Global)
			}
		}
	}
	return nil
}

// claudeAvailable and claudeMCP are the single seam for the Claude Code CLI.
// Claude global MCP registration goes through its own CLI (we never edit
// ~/.claude.json directly), and these vars let tests stub it so the suite never
// mutates the developer's real `claude mcp` registration.
var (
	claudeAvailable = func() bool {
		_, err := exec.LookPath("claude")
		return err == nil
	}
	claudeMCP = func(args ...string) ([]byte, error) {
		return exec.Command("claude", append([]string{"mcp"}, args...)...).CombinedOutput()
	}
)

// IsMCPGloballyRegistered reports whether lerd is registered with Claude Code.
// Uses `claude mcp get lerd` which returns exit 0 when the server is known and
// exit 1 otherwise. The older `claude mcp list --scope user` flag form breaks
// on newer Claude CLI releases.
func IsMCPGloballyRegistered() bool {
	if !claudeAvailable() {
		return false
	}
	_, err := claudeMCP("get", "lerd")
	return err == nil
}

// ensureClaudeMCPRegistered adds lerd to Claude Code at user scope only when
// `claude mcp get lerd` reports it missing. Add-only (no remove-then-add) so
// a failing add can't leave the user unregistered. No-op when claude isn't
// installed or lerd is already registered.
func ensureClaudeMCPRegistered() {
	if !claudeAvailable() {
		return
	}
	if _, err := claudeMCP("get", "lerd"); err == nil {
		return
	}
	if _, err := claudeMCP("add", "-s", "user", "lerd", "--", "lerd", "mcp"); err != nil {
		fmt.Printf("  [WARN] could not register lerd with Claude Code: %v\n", err)
		fmt.Println("  Run manually: claude mcp add -s user lerd -- lerd mcp")
	}
}

// mergeSentinelSection upserts the lerd section inside a shared markdown doc
// (Junie guidelines, GEMINI.md, AGENTS.md, copilot-instructions.md). If the
// file does not exist it is created. If a lerd section already exists (delimited
// by the sentinel comments) it is replaced; otherwise the section is appended.
func mergeSentinelSection(path, section string) error {
	const begin = "<!-- lerd:begin -->"
	const end = "<!-- lerd:end -->"

	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}

	block := begin + "\n" + section + "\n" + end

	if strings.Contains(existing, begin) {
		// Replace the existing lerd block.
		startIdx := strings.Index(existing, begin)
		endIdx := strings.Index(existing, end)
		if endIdx == -1 {
			// Malformed — replace from begin to EOF.
			existing = strings.TrimRight(existing[:startIdx], "\n") + "\n\n" + block + "\n"
		} else {
			existing = existing[:startIdx] + block + existing[endIdx+len(end):]
		}
	} else {
		// Append, ensuring a blank line separator.
		if existing != "" {
			existing = strings.TrimRight(existing, "\n") + "\n\n"
		}
		existing += block + "\n"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(existing), 0644)
}

// stripSentinelSection removes the lerd-delimited block from a shared markdown
// doc. Returns (changed, err). When the file is empty after the block is
// stripped it is removed.
func stripSentinelSection(path string) (bool, error) {
	const begin = "<!-- lerd:begin -->"
	const end = "<!-- lerd:end -->"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	s := string(data)
	startIdx := strings.Index(s, begin)
	if startIdx == -1 {
		return false, nil
	}
	endIdx := strings.Index(s, end)
	if endIdx == -1 {
		s = s[:startIdx]
	} else {
		s = s[:startIdx] + s[endIdx+len(end):]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return true, err
		}
		return true, nil
	}
	return true, os.WriteFile(path, []byte(s+"\n"), 0644)
}

// RemoveGlobalAISkills tears down every user-scope artefact written by the
// Write/RunMCPEnableGlobal path across the whole client registry: context docs,
// global MCP config entries, and the Claude Code user-scope registration.
func RemoveGlobalAISkills(home string, verbose bool) error {
	log := func(msg string) {
		if verbose {
			fmt.Println(msg)
		}
	}

	for _, c := range aiClients {
		if c.GlobalViaCLI {
			if _, err := claudeMCP("remove", "--scope", "user", "lerd"); err == nil {
				log("  removed Claude Code user-scope MCP registration")
			}
		}
		if c.GlobalMCP != "" {
			full := filepath.Join(home, c.GlobalMCP)
			if changed, err := removeClientMCP(full, c); err != nil {
				fmt.Printf("  warn: %s: %v\n", full, err)
			} else if changed {
				log("  cleaned " + full)
			}
		}
		for _, cx := range c.Contexts {
			if cx.Global == "" {
				continue
			}
			full := filepath.Join(home, cx.Global)
			if changed, err := removeClientContext(full, cx); err != nil {
				return err
			} else if changed {
				log("  removed " + full)
			}
		}
	}
	return nil
}

// RemoveProjectAISkills removes every lerd-owned artefact under abs across the
// client registry: project MCP config entries and context docs. Opt-out
// counterpart of WriteProjectAISkills.
func RemoveProjectAISkills(abs string, verbose bool) error {
	log := func(msg string) {
		if verbose {
			fmt.Println(msg)
		}
	}

	for _, c := range aiClients {
		if c.ProjectMCP != "" {
			full := filepath.Join(abs, c.ProjectMCP)
			if changed, err := removeClientMCP(full, c); err != nil {
				fmt.Printf("  warn: %s: %v\n", full, err)
			} else if changed {
				log("  cleaned " + full)
			}
		}
		for _, cx := range c.Contexts {
			if cx.Project == "" {
				continue
			}
			full := filepath.Join(abs, cx.Project)
			if changed, err := removeClientContext(full, cx); err != nil {
				return err
			} else if changed {
				log("  removed " + full)
			}
		}
	}
	return nil
}

// lerdReference is the single canonical lerd tool reference shared by every AI
// assistant. Each client wraps it with its own thin frontmatter (Claude SKILL,
// Cursor .mdc) or embeds it sentinel-wrapped (Junie, Codex, Gemini, Copilot),
// so the docs an assistant reads never drift between clients.
//
//go:embed aidocs/lerd-reference.md
var lerdReference string

const skillDescription = "Manage the lerd local PHP development environment — run framework console commands (artisan, bin/console, etc.), manage services, start/stop queue workers, run composer, manage Node.js versions, and inspect site status via MCP tools."

const cursorDescription = "Lerd local PHP development environment — use the lerd MCP tools to manage sites, services, workers, and PHP/Node runtimes."

// renderClaudeSkill wraps the reference with Claude Code skill frontmatter.
func renderClaudeSkill() string {
	return "---\nname: lerd\ndescription: " + skillDescription + "\n---\n" + lerdReference
}

// renderCursorRules wraps the reference with Cursor .mdc frontmatter.
func renderCursorRules() string {
	return "---\ndescription: " + cursorDescription + "\nglobs:\nalwaysApply: true\n---\n" + lerdReference
}
