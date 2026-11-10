package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/daemon"
	"github.com/geodro/lerd/internal/feedback"
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
			// Lives as long as the assistant session that spawned it, and a user
			// with several sessions open gets one of these per session.
			daemon.TuneRuntime()
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
  .gemini/settings.json            Gemini CLI MCP config
  GEMINI.md                        Gemini CLI context
  .vscode/mcp.json                 GitHub Copilot (VS Code) MCP config
  .github/copilot-instructions.md  GitHub Copilot instructions
  AGENTS.md                        Codex CLI context (also read by OpenCode)
  opencode.json                    OpenCode MCP config

Run this from a Laravel project root, or use --path to specify a directory.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMCPInject(targetPath)
		},
	}
	cmd.Flags().StringVar(&targetPath, "path", "", "Target project directory (defaults to current directory)")
	return cmd
}

// NewMCPEjectCmd returns the mcp:eject command, the inverse of mcp:inject.
func NewMCPEjectCmd() *cobra.Command {
	var targetPath string
	cmd := &cobra.Command{
		Use:   "mcp:eject",
		Short: "Remove lerd MCP config and AI skill files from a project",
		Long: `Removes every lerd-owned MCP server entry and context file that
mcp:inject wrote into the target project directory, leaving other MCP servers
(e.g. laravel-boost) and your own instructions content untouched. Also strips
the entry an older lerd left in .ai/mcp/mcp.json.

Run this from a project root, or use --path to specify a directory.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMCPEject(targetPath)
		},
	}
	cmd.Flags().StringVar(&targetPath, "path", "", "Target project directory (defaults to current directory)")
	return cmd
}

func runMCPEject(targetPath string) error {
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

	feedback.Begin()
	feedback.Line("removing lerd MCP config from " + feedback.Val(abs))
	if err := RemoveProjectAISkills(abs, true); err != nil {
		return err
	}
	feedback.Done("done — restart your AI assistant to unload the lerd MCP server")
	return nil
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

	feedback.Begin()
	feedback.Line("injecting lerd MCP config into " + feedback.Val(abs))
	if err := WriteProjectAISkills(abs, true); err != nil {
		return err
	}
	feedback.Done("done — restart your AI assistant to load the lerd MCP server")
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
// A project that set mcp_inject: false in .lerd.yaml is skipped entirely so
// lerd never touches its committed config on a self-update.
func RefreshProjectAISkills(abs string, verbose bool) error {
	if cfg, err := config.LoadProjectConfig(abs); err == nil && cfg.MCPInjectDisabled() {
		return nil
	}
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
				if err := writeClientMCP(full, c); err != nil {
					return err
				}
				// Report the file that was written, which is not always the one
				// the client declares: an existing opencode.jsonc is merged into
				// rather than shadowed by a sibling .json.
				log("  updated " + filepath.Base(resolveClientConfigPath(full)))
			}
		}
		for _, cx := range clientContexts(c) {
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
		for _, cx := range staleSkillContexts(c) {
			if changed, _ := removeClientContext(filepath.Join(abs, cx.Project), cx); changed {
				log("  removed " + cx.Project)
			}
		}
	}

	if sweepLegacySharedAIMCP(abs) {
		log("  cleaned " + legacySharedAIMCP + " (no longer written)")
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
  ~/.codeium/windsurf/mcp_config.json  Windsurf global MCP config
  ~/.junie/mcp/mcp.json            JetBrains Junie global MCP config
  ~/.gemini/settings.json          Gemini CLI global MCP config
  ~/.codex/config.toml             Codex CLI global MCP config
  ~/.config/Code/User/mcp.json     GitHub Copilot (VS Code) global MCP config
  ~/.gemini/config/mcp_config.json Google Antigravity global MCP config
  ~/.config/opencode/opencode.json OpenCode global MCP config
  ~/.claude/skills/lerd/SKILL.md   Claude Code user-scope skill
  ~/.cursor/rules/lerd.mdc         Cursor user-scope rules
  ~/.junie/guidelines.md           JetBrains Junie user-scope guidelines
  ~/.gemini/GEMINI.md              Gemini CLI user-scope context
  ~/.codex/AGENTS.md               Codex CLI user-scope context
  ~/.config/opencode/AGENTS.md     OpenCode user-scope context`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return RunMCPEnableGlobal()
		},
	}
}

// RunMCPEnableGlobal registers lerd MCP at user scope for all supported AI tools.
// It is exported so the install command can call it directly.
func RunMCPEnableGlobal() error {
	feedback.Begin()
	feedback.Line("registering lerd MCP globally")

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
	enableWindowsAIClients(true)

	feedback.Done("done — restart your AI assistant for changes to take effect")
	feedback.Note("lerd uses the directory you open Claude in as the site context")
	return nil
}

// NewMCPDisableGlobalCmd returns the mcp:disable-global command, the inverse of
// mcp:enable-global.
func NewMCPDisableGlobalCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp:disable-global",
		Short: "Unregister the user-scope lerd MCP server for all AI assistants",
		Long: `Removes the user-scope lerd MCP registration and context docs written
by mcp:enable-global: the Claude Code user-scope entry and lerd-php plugin, each client's global MCP
config, and the user-scope skill/guidelines files. Other MCP servers and your own
instructions content are left untouched.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return RunMCPDisableGlobal()
		},
	}
}

// RunMCPDisableGlobal tears down the user-scope lerd MCP registration.
func RunMCPDisableGlobal() error {
	feedback.Begin()
	feedback.Line("unregistering lerd MCP globally")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := RemoveGlobalAISkills(home, true); err != nil {
		return err
	}
	removeWindowsAIClients(true)
	saveGlobalMCPDeclined(true)

	feedback.Done("done — restart your AI assistant for changes to take effect")
	return nil
}

// writeGlobalMCPConfigs registers the user-scope MCP server for every client in
// the registry: file-backed clients via their global config, Claude via the
// idempotent `claude mcp add` CLI. Global entries omit LERD_SITE_PATH so the
// server uses the directory the assistant is opened in.
func writeGlobalMCPConfigs(home string, verbose bool) error {
	return writeHomeMCPConfigs(localAIHome(home), verbose)
}

// aiHome is where user-scope registration goes: this machine's home, or on WSL
// the Windows profile, whose clients reach lerd through lerd.exe.
type aiHome struct {
	dir       string                               // the clients' home folder
	command   string                               // what a registered entry runs
	claude    func(args ...string) ([]byte, error) // Claude Code's `mcp` CLI
	claudeDir string                               // CLAUDE_CONFIG_DIR, "" for ~/.claude
	windows   bool                                 // a Windows profile: installed clients only
}

func localAIHome(home string) aiHome {
	return aiHome{
		dir:       home,
		command:   "lerd",
		claude:    func(args ...string) ([]byte, error) { return claudeMCP(args...) },
		claudeDir: os.Getenv("CLAUDE_CONFIG_DIR"),
	}
}

// has reports whether lerd registers with c here. A Windows profile only gets
// the clients installed in it, so lerd does not litter it with config folders.
func (h aiHome) has(c aiClient) bool {
	if !h.windows {
		return true
	}
	if c.WindowsDir == "" {
		return false
	}
	// contextPath, so a Claude profile under CLAUDE_CONFIG_DIR counts as installed.
	_, err := os.Stat(h.contextPath(c.WindowsDir))
	return err == nil
}

func (h aiHome) mcpPath(c aiClient) string {
	rel := c.GlobalMCP
	if h.windows && c.WindowsMCP != "" {
		rel = c.WindowsMCP
	}
	if rel == "" {
		return ""
	}
	return filepath.Join(h.dir, rel)
}

// contextPath resolves a client's user-scope doc, which Claude Code reads from
// CLAUDE_CONFIG_DIR when a profile sets it.
func (h aiHome) contextPath(rel string) string {
	claudeRel, err := filepath.Rel(".claude", rel)
	if h.claudeDir == "" || err != nil || strings.HasPrefix(claudeRel, "..") {
		return filepath.Join(h.dir, rel)
	}
	return filepath.Join(h.claudeDir, claudeRel)
}

func writeHomeMCPConfigs(h aiHome, verbose bool) error {
	log := func(msg string) {
		if verbose {
			feedback.Note(msg)
		}
	}
	for _, c := range aiClients {
		if !h.has(c) {
			continue
		}
		if c.GlobalViaCLI {
			// Try remove first for idempotent re-registration, then add.
			_, _ = h.claude("remove", "--scope", "user", "lerd")
			out, err := h.claude("add", "--scope", "user", "lerd", "--", h.command, "mcp")
			switch {
			case errors.Is(err, errNoClaude):
			case err != nil:
				feedback.Warn("could not register with Claude Code (%v): %s", err, strings.TrimSpace(string(out)))
				feedback.Note("run manually: claude mcp add --scope user lerd -- " + h.command + " mcp")
			default:
				log("registered in Claude Code (user scope)")
				// lerd lsp runs on this machine only, so a Windows profile gets no plugin.
				if !h.windows {
					installClaudeLSPPlugin(log)
				}
			}
			continue
		}
		path := h.mcpPath(c)
		if path == "" {
			continue
		}
		if err := writeClientMCPCommand(path, c, h.command); err != nil {
			return err
		}
		log("updated " + resolveClientConfigPath(path))
	}
	if !h.windows && sweepLegacySharedAIMCP(h.dir) {
		log("cleaned ~/" + legacySharedAIMCP + " (no longer written)")
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

// globalContextPath resolves a client's user-scope doc under home, except that
// Claude Code reads its config from CLAUDE_CONFIG_DIR when a profile sets it.
func globalContextPath(home, rel string) string {
	return localAIHome(home).contextPath(rel)
}

func writeGlobalContexts(home string, verbose, createMissing bool) error {
	return writeHomeContexts(localAIHome(home), verbose, createMissing)
}

func writeHomeContexts(h aiHome, verbose, createMissing bool) error {
	for _, c := range aiClients {
		if !h.has(c) {
			continue
		}
		for _, cx := range clientContexts(c) {
			if cx.Global == "" {
				continue
			}
			full := h.contextPath(cx.Global)
			if !createMissing && !contextAlreadyPresent(full, cx) {
				continue
			}
			if err := writeClientContext(full, cx); err != nil {
				return fmt.Errorf("writing %s: %w", cx.Global, err)
			}
			if verbose {
				feedback.Note("wrote " + full)
			}
		}
		for _, cx := range staleSkillContexts(c) {
			full := h.contextPath(cx.Global)
			if changed, _ := removeClientContext(full, cx); changed && verbose {
				feedback.Note("removed " + full)
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
	claudePlugin = func(args ...string) ([]byte, error) {
		return exec.Command("claude", append([]string{"plugin"}, args...)...).CombinedOutput()
	}
)

// Claude Code takes a language server only from a plugin, so lerd's PHP one ships
// as lerd-php in its own small marketplace repo, which is all the CLI downloads.
const (
	lerdClaudeMarketplaceRepo = "lerd-env/claude-plugins"
	lerdClaudeMarketplace     = "lerd"
	lerdClaudeLSPPlugin       = "lerd-php@" + lerdClaudeMarketplace
)

// installClaudeLSPPlugin ignores the marketplace add, which fails when a previous
// enable already added it; the install alone decides whether a warning is due.
func installClaudeLSPPlugin(log func(string)) {
	_, _ = claudePlugin("marketplace", "add", lerdClaudeMarketplaceRepo)
	if out, err := claudePlugin("install", "--scope", "user", lerdClaudeLSPPlugin); err != nil {
		feedback.Warn("could not install the lerd-php plugin in Claude Code (%v): %s", err, strings.TrimSpace(string(out)))
		feedback.Note("run manually: claude plugin marketplace add " + lerdClaudeMarketplaceRepo + " && claude plugin install " + lerdClaudeLSPPlugin)
		return
	}
	log("installed the lerd-php language server plugin in Claude Code")
}

// removeClaudeLSPPlugin stays quiet when there is nothing to remove: the CLI says
// "not found" for a plugin or marketplace already gone, and for a missing claude.
func removeClaudeLSPPlugin(log func(string)) {
	out, err := claudePlugin("uninstall", "--scope", "user", lerdClaudeLSPPlugin)
	switch {
	case err == nil:
		log("  removed the lerd-php plugin from Claude Code")
	case !strings.Contains(string(out)+err.Error(), "not found"):
		feedback.Warn("could not remove the lerd-php plugin from Claude Code (%v): %s", err, strings.TrimSpace(string(out)))
		feedback.Note("run manually: claude plugin uninstall --scope user " + lerdClaudeLSPPlugin)
	}
	out, err = claudePlugin("marketplace", "remove", lerdClaudeMarketplace)
	if err != nil && !strings.Contains(string(out)+err.Error(), "not found") {
		feedback.Warn("could not remove the lerd marketplace from Claude Code (%v): %s", err, strings.TrimSpace(string(out)))
	}
}

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
		feedback.Warn("could not register lerd with Claude Code: %v", err)
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
	return removeHomeAI(localAIHome(home), verbose)
}

// removeHomeAI takes lerd back out of every client in h, installed or not, since
// one removed after lerd registered with it can still hold the entry.
func removeHomeAI(h aiHome, verbose bool) error {
	log := func(msg string) {
		if verbose {
			fmt.Println(msg)
		}
	}

	for _, c := range aiClients {
		if c.GlobalViaCLI {
			if _, err := h.claude("remove", "--scope", "user", "lerd"); err == nil {
				log("  removed Claude Code user-scope MCP registration")
			}
			if !h.windows {
				removeClaudeLSPPlugin(log)
			}
		}
		if full := h.mcpPath(c); full != "" {
			if changed, err := removeClientMCP(full, c); err != nil {
				fmt.Printf("  warn: %s: %v\n", full, err)
			} else if changed {
				log("  cleaned " + resolveClientConfigPath(full))
			}
		}
		for _, cx := range removableContexts(c) {
			if cx.Global == "" {
				continue
			}
			full := h.contextPath(cx.Global)
			if changed, err := removeClientContext(full, cx); err != nil {
				return err
			} else if changed {
				log("  removed " + full)
			}
		}
	}
	if !h.windows && sweepLegacySharedAIMCP(h.dir) {
		log("  cleaned ~/" + legacySharedAIMCP)
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
				log("  cleaned " + resolveClientConfigPath(full))
			}
		}
		for _, cx := range removableContexts(c) {
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
	if sweepLegacySharedAIMCP(abs) {
		log("  cleaned " + legacySharedAIMCP)
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

const skillDescription = "Manage the lerd local PHP development environment via MCP tools: run framework console commands (artisan, bin/console, etc.), manage services, start/stop queue workers, run composer, create and remove git worktrees, manage Node.js versions, and inspect site status. Also the way to diagnose and optimize a slow site: find N+1 and slow queries, read per-site response-time and slow-route timings, profile requests, and run site health checks, from real captured traffic rather than reading code. And the way to see why a page breaks in the browser: read the JavaScript errors and console warnings its pages report (browser logs)."

const cursorDescription = "Lerd local PHP development environment — use the lerd MCP tools to manage sites, services, workers, and PHP/Node runtimes, and to diagnose slow sites (N+1 and slow queries, request timing, slow routes, profiling) from real traffic, and to read the JavaScript errors and console warnings a site's pages report."

// renderClaudeSkill wraps the reference with Claude Code skill frontmatter.
func renderClaudeSkill() string {
	return "---\nname: lerd\ndescription: " + skillDescription + "\n---\n" + lerdReference
}

// renderCursorRules wraps the reference with Cursor .mdc frontmatter.
func renderCursorRules() string {
	return "---\ndescription: " + cursorDescription + "\nglobs:\nalwaysApply: true\n---\n" + lerdReference
}
