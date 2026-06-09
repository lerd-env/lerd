package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// TestMain stubs the Claude Code CLI seam so the cli test suite never mutates the
// developer's real `claude mcp` registration (RemoveGlobalAISkills and the
// enable-global path otherwise shell out to the live binary).
func TestMain(m *testing.M) {
	claudeAvailable = func() bool { return false }
	claudeMCP = func(args ...string) ([]byte, error) { return nil, nil }
	os.Exit(m.Run())
}

// TestCopilotUsesServersKey verifies the VS Code config quirk: the top-level key
// is "servers" (not "mcpServers") and each entry carries "type":"stdio".
func TestCopilotUsesServersKey(t *testing.T) {
	dir := t.TempDir()
	if err := WriteProjectAISkills(dir, false); err != nil {
		t.Fatalf("WriteProjectAISkills: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".vscode", "mcp.json"))
	if err != nil {
		t.Fatalf("read .vscode/mcp.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .vscode/mcp.json: %v", err)
	}
	if _, bad := cfg["mcpServers"]; bad {
		t.Errorf("VS Code config must not use mcpServers key: %s", data)
	}
	servers, ok := cfg["servers"].(map[string]any)
	if !ok {
		t.Fatalf("VS Code config missing servers key: %s", data)
	}
	lerd, ok := servers["lerd"].(map[string]any)
	if !ok {
		t.Fatalf("servers.lerd missing: %s", data)
	}
	if lerd["type"] != "stdio" {
		t.Errorf("servers.lerd.type should be stdio, got %v", lerd["type"])
	}
	if lerd["command"] != "lerd" {
		t.Errorf("servers.lerd.command should be lerd, got %v", lerd["command"])
	}
}

// TestGeminiUsesMcpServersKey confirms Gemini's settings.json keeps the standard
// mcpServers key (a regression guard alongside the VS Code servers quirk).
func TestGeminiUsesMcpServersKey(t *testing.T) {
	dir := t.TempDir()
	if err := WriteProjectAISkills(dir, false); err != nil {
		t.Fatalf("WriteProjectAISkills: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("read .gemini/settings.json: %v", err)
	}
	if !strings.Contains(string(data), `"mcpServers"`) {
		t.Errorf("gemini settings.json should use mcpServers key: %s", data)
	}
}

// TestAntigravityGlobalConfig confirms Antigravity registers at its HOME-level
// path with the mcpServers key and no stdio type field, and that it has no
// project-scoped MCP config (its project scope is a no-op).
func TestAntigravityGlobalConfig(t *testing.T) {
	home := t.TempDir()
	var ag aiClient
	for _, c := range aiClients {
		if c.Name == "antigravity" {
			ag = c
		}
	}
	if ag.Name == "" {
		t.Fatal("antigravity client not registered")
	}
	if ag.ProjectMCP != "" {
		t.Errorf("antigravity should be global-only, got ProjectMCP=%q", ag.ProjectMCP)
	}
	if err := writeClientMCP(filepath.Join(home, ag.GlobalMCP), ag, ""); err != nil {
		t.Fatalf("writeClientMCP: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".gemini", "config", "mcp_config.json"))
	if err != nil {
		t.Fatalf("read antigravity config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	lerd, ok := servers["lerd"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers.lerd missing: %s", data)
	}
	if _, hasType := lerd["type"]; hasType {
		t.Errorf("antigravity entry should not carry a type field: %s", data)
	}
}

func TestMergeCodexTOML_createsAndPreservesOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	seed := "model = \"o4\"\n\n[mcp_servers.other]\ncommand = \"x\"\n"
	if err := os.WriteFile(path, []byte(seed), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := mergeCodexTOML(path); err != nil {
		t.Fatalf("mergeCodexTOML: %v", err)
	}

	data, _ := os.ReadFile(path)
	doc := map[string]any{}
	if err := toml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc["model"] != "o4" {
		t.Errorf("top-level model key was lost: %s", data)
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Errorf("unrelated mcp_servers.other was dropped: %s", data)
	}
	lerd, ok := servers["lerd"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers.lerd not added: %s", data)
	}
	if lerd["command"] != "lerd" {
		t.Errorf("lerd.command should be lerd, got %v", lerd["command"])
	}
}

func TestMergeCodexTOML_idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := mergeCodexTOML(path); err != nil {
		t.Fatalf("first: %v", err)
	}
	first, _ := os.ReadFile(path)
	if err := mergeCodexTOML(path); err != nil {
		t.Fatalf("second: %v", err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("mergeCodexTOML not idempotent:\n%s\n---\n%s", first, second)
	}
}

func TestRemoveCodexTOML_removesOnlyLerd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	seed := "[mcp_servers.lerd]\ncommand = \"lerd\"\n\n[mcp_servers.other]\ncommand = \"x\"\n"
	if err := os.WriteFile(path, []byte(seed), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	changed, err := removeCodexTOML(path)
	if err != nil {
		t.Fatalf("removeCodexTOML: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "[mcp_servers.lerd]") {
		t.Errorf("lerd entry should be gone: %s", data)
	}
	if !strings.Contains(string(data), "[mcp_servers.other]") {
		t.Errorf("other entry was dropped: %s", data)
	}
}

func TestRemoveCodexTOML_deletesFileWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp_servers.lerd]\ncommand = \"lerd\"\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	changed, err := removeCodexTOML(path)
	if err != nil {
		t.Fatalf("removeCodexTOML: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("empty config.toml should be removed, err=%v", err)
	}
}

// TestCopilotInstructionsPreserveUserContent confirms the sentinel merge keeps a
// user's existing copilot-instructions.md content when the lerd block is added.
func TestCopilotInstructionsPreserveUserContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".github", "copilot-instructions.md")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("# House rules\n\nUse tabs.\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := WriteProjectAISkills(dir, false); err != nil {
		t.Fatalf("WriteProjectAISkills: %v", err)
	}

	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "Use tabs.") {
		t.Errorf("user instructions were dropped:\n%s", got)
	}
	if !strings.Contains(string(got), "<!-- lerd:begin -->") {
		t.Errorf("lerd block not added:\n%s", got)
	}
}

// TestWriteGlobalMCPConfigs_writesFileBackedClients checks the global MCP config
// path for the file-backed clients. It deliberately bypasses writeGlobalMCPConfigs
// (which would shell out to the real `claude mcp add` CLI and mutate this
// machine's actual registration) by driving writeClientMCP per client.
func TestWriteGlobalMCPConfigs_writesFileBackedClients(t *testing.T) {
	home := t.TempDir()
	for _, c := range aiClients {
		if c.GlobalViaCLI || c.GlobalMCP == "" {
			continue
		}
		if err := writeClientMCP(filepath.Join(home, c.GlobalMCP), c, ""); err != nil {
			t.Fatalf("writeClientMCP(%s): %v", c.Name, err)
		}
	}
	for _, rel := range []string{
		filepath.Join(".cursor", "mcp.json"),
		filepath.Join(".ai", "mcp", "mcp.json"),
		filepath.Join(".junie", "mcp", "mcp.json"),
		filepath.Join(".gemini", "settings.json"),
		filepath.Join(".codex", "config.toml"),
		filepath.Join(".config", "Code", "User", "mcp.json"),
		filepath.Join(".gemini", "config", "mcp_config.json"),
	} {
		if _, err := os.Stat(filepath.Join(home, rel)); err != nil {
			t.Errorf("global MCP config %s missing: %v", rel, err)
		}
	}
	// Global JSON entries must not carry LERD_SITE_PATH.
	data, _ := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
	if strings.Contains(string(data), "LERD_SITE_PATH") {
		t.Errorf("global entry should not pin LERD_SITE_PATH: %s", data)
	}
}

// TestRefreshGlobalAISkills_onlyTouchesExisting confirms `lerd update` does not
// expand a user's footprint: a global user who only had the Claude skill keeps
// it refreshed but does not get ~/.gemini/GEMINI.md or ~/.codex/AGENTS.md created.
func TestRefreshGlobalAISkills_onlyTouchesExisting(t *testing.T) {
	home := t.TempDir()
	skill := filepath.Join(home, ".claude", "skills", "lerd", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(skill, []byte("stale"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RefreshGlobalAISkills(home, false); err != nil {
		t.Fatalf("RefreshGlobalAISkills: %v", err)
	}

	if got, _ := os.ReadFile(skill); string(got) == "stale" {
		t.Error("existing SKILL.md was not refreshed")
	}
	for _, rel := range []string{
		filepath.Join(".gemini", "GEMINI.md"),
		filepath.Join(".codex", "AGENTS.md"),
		filepath.Join(".cursor", "rules", "lerd.mdc"),
	} {
		if _, err := os.Stat(filepath.Join(home, rel)); !os.IsNotExist(err) {
			t.Errorf("refresh created %s for a client the user never enabled (err=%v)", rel, err)
		}
	}
}

// TestRefreshGlobalAISkills_skipsForeignSentinelFile confirms refresh does not
// adopt a user's own AGENTS.md/GEMINI.md that lacks the lerd sentinel block.
func TestRefreshGlobalAISkills_skipsForeignSentinelFile(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agents), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(agents, []byte("# my own codex notes\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := RefreshGlobalAISkills(home, false); err != nil {
		t.Fatalf("RefreshGlobalAISkills: %v", err)
	}

	got, _ := os.ReadFile(agents)
	if strings.Contains(string(got), "<!-- lerd:begin -->") {
		t.Errorf("refresh injected a lerd block into a user's own AGENTS.md:\n%s", got)
	}
}

// TestRefreshProjectAISkills_onlyTouchesExisting confirms project refresh updates
// only the clients already set up and does not add new client files to the repo.
func TestRefreshProjectAISkills_onlyTouchesExisting(t *testing.T) {
	dir := t.TempDir()
	// Simulate an old opt-in: Claude MCP config + skill present, nothing else.
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"),
		[]byte(`{"mcpServers":{"lerd":{"command":"lerd","args":["mcp"]}}}`), 0644); err != nil {
		t.Fatalf("seed mcp.json: %v", err)
	}
	skill := filepath.Join(dir, ".claude", "skills", "lerd", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(skill, []byte("stale"), 0644); err != nil {
		t.Fatalf("seed skill: %v", err)
	}

	if err := RefreshProjectAISkills(dir, false); err != nil {
		t.Fatalf("RefreshProjectAISkills: %v", err)
	}

	if got, _ := os.ReadFile(skill); string(got) == "stale" {
		t.Error("existing SKILL.md was not refreshed")
	}
	for _, rel := range []string{".vscode/mcp.json", ".gemini/settings.json", "GEMINI.md", "AGENTS.md", ".github/copilot-instructions.md", ".cursor/mcp.json"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Errorf("project refresh created %s for a client never opted into (err=%v)", rel, err)
		}
	}
}

// TestProjectMCP_carriesSitePath confirms project-scoped JSON entries pin the
// site directory via LERD_SITE_PATH.
func TestProjectMCP_carriesSitePath(t *testing.T) {
	dir := t.TempDir()
	if err := WriteProjectAISkills(dir, false); err != nil {
		t.Fatalf("WriteProjectAISkills: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if !strings.Contains(string(data), "LERD_SITE_PATH") {
		t.Errorf("project entry should pin LERD_SITE_PATH: %s", data)
	}
	if !strings.Contains(string(data), dir) {
		t.Errorf("project entry should reference the site dir %s: %s", dir, data)
	}
}
