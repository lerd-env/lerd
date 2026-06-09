package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// mcpFormat selects how a client's MCP server config file is written.
type mcpFormat int

const (
	fmtJSONMcpServers mcpFormat = iota // {"mcpServers": {...}} — Claude, Cursor, Junie, Windsurf, Gemini
	fmtJSONServers                     // {"servers": {...}} with "type":"stdio" — VS Code / Copilot
	fmtTOMLCodex                       // [mcp_servers.lerd] in ~/.codex/config.toml
)

// ctxFormat selects how a client's context/instructions doc is written.
type ctxFormat int

const (
	ctxOverwrite ctxFormat = iota // lerd owns the whole file (SKILL.md, lerd.mdc)
	ctxSentinel                   // upsert the lerd block between sentinel comments
)

// ctxFile describes one context/instructions document a client auto-loads.
// Project and Global are paths relative to the project root and $HOME; an empty
// value means the file is not written at that scope. Content yields the bytes to
// write — a func so each client can wrap the shared embedded reference with its
// own frontmatter (renderClaudeSkill / renderCursorRules) lazily.
type ctxFile struct {
	Project string
	Global  string
	Format  ctxFormat
	Content func() string
}

// aiClient is one supported AI coding assistant and where lerd registers with it.
type aiClient struct {
	Name         string    // identifier, used only in log lines
	ProjectMCP   string    // MCP config path rel to project root; "" = no project scope (Codex)
	GlobalMCP    string    // MCP config path rel to $HOME; "" = none (Claude uses CLI)
	MCPFormat    mcpFormat // how the MCP config file is encoded
	ServerKey    string    // top-level JSON key: "mcpServers" or "servers"
	NeedsType    bool      // inject "type":"stdio" into the entry (VS Code only)
	GlobalViaCLI bool      // register global scope via `claude mcp add` instead of a file
	Contexts     []ctxFile // context/instructions docs this client loads
}

// aiClients is the single source of truth for every assistant lerd integrates
// with. Adding a client is a table entry, not new write/remove code. Claude
// global MCP is the one file-write exception: ~/.claude.json holds all Claude
// user state with an undocumented schema, so we register via the idempotent
// `claude mcp add` CLI rather than risk corrupting it.
var aiClients = []aiClient{
	{
		Name:       "claude",
		ProjectMCP: ".mcp.json",
		// Global registration is handled via the Claude CLI, not a file.
		MCPFormat:    fmtJSONMcpServers,
		ServerKey:    "mcpServers",
		GlobalViaCLI: true,
		Contexts: []ctxFile{{
			Project: filepath.Join(".claude", "skills", "lerd", "SKILL.md"),
			Global:  filepath.Join(".claude", "skills", "lerd", "SKILL.md"),
			Format:  ctxOverwrite,
			Content: renderClaudeSkill,
		}},
	},
	{
		Name:       "cursor",
		ProjectMCP: filepath.Join(".cursor", "mcp.json"),
		GlobalMCP:  filepath.Join(".cursor", "mcp.json"),
		MCPFormat:  fmtJSONMcpServers,
		ServerKey:  "mcpServers",
		Contexts: []ctxFile{{
			Project: filepath.Join(".cursor", "rules", "lerd.mdc"),
			Global:  filepath.Join(".cursor", "rules", "lerd.mdc"),
			Format:  ctxOverwrite,
			Content: renderCursorRules,
		}},
	},
	{
		Name:       "junie",
		ProjectMCP: filepath.Join(".junie", "mcp", "mcp.json"),
		GlobalMCP:  filepath.Join(".junie", "mcp", "mcp.json"),
		MCPFormat:  fmtJSONMcpServers,
		ServerKey:  "mcpServers",
		Contexts: []ctxFile{{
			Project: filepath.Join(".junie", "guidelines.md"),
			Global:  filepath.Join(".junie", "guidelines.md"),
			Format:  ctxSentinel,
			Content: func() string { return lerdReference },
		}},
	},
	{
		Name:       "windsurf",
		ProjectMCP: filepath.Join(".ai", "mcp", "mcp.json"),
		GlobalMCP:  filepath.Join(".ai", "mcp", "mcp.json"),
		MCPFormat:  fmtJSONMcpServers,
		ServerKey:  "mcpServers",
	},
	{
		Name: "codex",
		// Codex has no project-scope MCP config; only ~/.codex/config.toml.
		GlobalMCP: filepath.Join(".codex", "config.toml"),
		MCPFormat: fmtTOMLCodex,
		Contexts: []ctxFile{{
			Project: "AGENTS.md",
			Global:  filepath.Join(".codex", "AGENTS.md"),
			Format:  ctxSentinel,
			Content: func() string { return lerdReference },
		}},
	},
	{
		Name:       "gemini",
		ProjectMCP: filepath.Join(".gemini", "settings.json"),
		GlobalMCP:  filepath.Join(".gemini", "settings.json"),
		MCPFormat:  fmtJSONMcpServers,
		ServerKey:  "mcpServers",
		Contexts: []ctxFile{{
			Project: "GEMINI.md",
			Global:  filepath.Join(".gemini", "GEMINI.md"),
			Format:  ctxSentinel,
			Content: func() string { return lerdReference },
		}},
	},
	{
		Name:       "copilot",
		ProjectMCP: filepath.Join(".vscode", "mcp.json"),
		GlobalMCP:  filepath.Join(".config", "Code", "User", "mcp.json"),
		MCPFormat:  fmtJSONServers,
		ServerKey:  "servers",
		NeedsType:  true,
		Contexts: []ctxFile{{
			// VS Code has no fixed global instructions file; project only.
			Project: filepath.Join(".github", "copilot-instructions.md"),
			Format:  ctxSentinel,
			Content: func() string { return lerdReference },
		}},
	},
	{
		Name: "antigravity",
		// Antigravity reads only the HOME-level MCP config; its project scope is
		// a known no-op, so register globally only. No Contexts: it auto-loads
		// GEMINI.md and AGENTS.md, which the gemini and codex entries already write.
		GlobalMCP: filepath.Join(".gemini", "config", "mcp_config.json"),
		MCPFormat: fmtJSONMcpServers,
		ServerKey: "mcpServers",
	},
}

// lerdJSONEntry builds the JSON MCP server entry. sitePath, when non-empty, is
// written as LERD_SITE_PATH so a project-scoped server pins to its directory;
// global entries omit it and the server falls back to the cwd at runtime.
func lerdJSONEntry(needsType bool, sitePath string) map[string]any {
	entry := map[string]any{"command": "lerd", "args": []string{"mcp"}}
	if needsType {
		entry["type"] = "stdio"
	}
	if sitePath != "" {
		entry["env"] = map[string]string{"LERD_SITE_PATH": sitePath}
	}
	return entry
}

// writeClientMCP writes/merges the lerd MCP entry into a client's config file,
// creating parent directories as needed.
func writeClientMCP(path string, c aiClient, sitePath string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if c.MCPFormat == fmtTOMLCodex {
		return mergeCodexTOML(path)
	}
	return mergeServerJSON(path, c.ServerKey, lerdJSONEntry(c.NeedsType, sitePath))
}

// writeClientContext writes a context/instructions doc per its format.
func writeClientContext(path string, cx ctxFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if cx.Format == ctxSentinel {
		return mergeSentinelSection(path, cx.Content())
	}
	return writeIfChanged(path, []byte(cx.Content()))
}

// mcpConfigHasLerd reports whether a client's JSON MCP config already registers
// lerd. Used by the refresh path to update only configs the user opted into,
// rather than expanding their footprint with new client files on every update.
func mcpConfigHasLerd(path string, c aiClient) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	cfg := map[string]any{}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	servers, _ := cfg[c.ServerKey].(map[string]any)
	_, ok := servers["lerd"]
	return ok
}

// contextAlreadyPresent reports whether a client's context doc already carries
// lerd content: an overwrite-owned file simply exists; a sentinel-shared file
// (guidelines.md, AGENTS.md, …) must contain the lerd block, so a user's own
// unrelated AGENTS.md is not adopted on refresh.
func contextAlreadyPresent(path string, cx ctxFile) bool {
	if cx.Format == ctxSentinel {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), "<!-- lerd:begin -->")
	}
	_, err := os.Stat(path)
	return err == nil
}

// removeClientMCP drops the lerd entry from a client's MCP config file.
func removeClientMCP(path string, c aiClient) (bool, error) {
	if c.MCPFormat == fmtTOMLCodex {
		return removeCodexTOML(path)
	}
	return removeServerJSON(path, c.ServerKey, "lerd")
}

// removeClientContext removes the lerd context doc: sentinel files keep user
// content, overwrite files are deleted outright (with empty-parent cleanup).
func removeClientContext(path string, cx ctxFile) (bool, error) {
	if cx.Format == ctxSentinel {
		return stripSentinelSection(path)
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_ = os.Remove(filepath.Dir(path))
	return true, nil
}

// mergeServerJSON reads an existing JSON config (if present), upserts the "lerd"
// key under serverKey, and writes it back indented. Unrelated servers and any
// other top-level keys are preserved.
func mergeServerJSON(path, serverKey string, entry map[string]any) error {
	cfg := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	servers, _ := cfg[serverKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers["lerd"] = entry
	cfg[serverKey] = servers

	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", path, err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// removeServerJSON drops the named server from serverKey in a shared JSON config.
// Returns (changed, err). Missing file/entry is a no-op; an emptied file is
// deleted entirely.
func removeServerJSON(path, serverKey, name string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	cfg := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return false, fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	servers, _ := cfg[serverKey].(map[string]any)
	if _, exists := servers[name]; !exists {
		return false, nil
	}
	delete(servers, name)
	if len(servers) == 0 {
		delete(cfg, serverKey)
	} else {
		cfg[serverKey] = servers
	}
	if len(cfg) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return true, err
		}
		return true, nil
	}
	out, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0644)
}

// codexLerdHeader is the TOML table header for lerd's Codex MCP entry.
const codexLerdHeader = "[mcp_servers.lerd]"

// mergeCodexTOML appends the [mcp_servers.lerd] table to ~/.codex/config.toml if
// it isn't already present. It edits the file textually rather than round-tripping
// through a TOML decoder/encoder, because go-toml strips comments, reorders
// tables, and rewrites quote styles — destructive for a hand-maintained file lerd
// does not own. lerd's entry never changes, so once present it is left untouched.
func mergeCodexTOML(path string) error {
	block := codexLerdHeader + "\ncommand = \"lerd\"\nargs = [\"mcp\"]\n"

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := string(data)
	if strings.Contains(existing, codexLerdHeader) {
		return nil
	}

	out := block
	if strings.TrimSpace(existing) != "" {
		out = strings.TrimRight(existing, "\n") + "\n\n" + block
	}
	return writeIfChanged(path, []byte(out))
}

// removeCodexTOML strips the [mcp_servers.lerd] table (header through the line
// before the next table, or EOF) from config.toml, leaving every other table and
// any comments intact. Returns (changed, err). Missing file/entry is a no-op; an
// emptied file is deleted.
func removeCodexTOML(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	s := string(data)
	idx := strings.Index(s, codexLerdHeader)
	if idx == -1 {
		return false, nil
	}
	// Block ends at the next table header ("\n[") or EOF.
	rest := s[idx+len(codexLerdHeader):]
	end := len(s)
	if j := strings.Index(rest, "\n["); j != -1 {
		end = idx + len(codexLerdHeader) + j + 1 // position of the next '['
	}
	out := strings.TrimSpace(s[:idx] + s[end:])
	if out == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return true, err
		}
		return true, nil
	}
	return true, os.WriteFile(path, []byte(out+"\n"), 0644)
}
