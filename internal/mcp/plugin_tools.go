package mcp

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// pluginTool is a plugin's MCP tool as tools/list advertises it. The schema is
// the plugin's own, passed through untouched, so it is not an mcpSchema.
type pluginTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// allTools is the built-in manifest followed by every installed plugin's tools,
// named <plugin>_<tool>. Plugins stay out of toolList so the built-in manifest,
// and the docs that are checked against it, never depend on what is installed.
func allTools() []any {
	tools := make([]any, 0, len(toolList()))
	for _, t := range toolList() {
		tools = append(tools, t)
	}
	list, _ := plugins.Load()
	for _, p := range list {
		if p.MCP == nil {
			continue
		}
		for _, t := range p.MCP.Tools {
			schema := t.InputSchema
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, pluginTool{Name: p.Name + "_" + t.Name, Description: t.Description, InputSchema: schema})
		}
	}
	return tools
}

// findPluginTool resolves a <plugin>_<tool> name to the installed plugin tool.
func findPluginTool(name string) (*plugins.Plugin, *plugins.MCPTool) {
	list, _ := plugins.Load()
	for _, p := range list {
		if p.MCP == nil || !strings.HasPrefix(name, p.Name+"_") {
			continue
		}
		for i, t := range p.MCP.Tools {
			if p.Name+"_"+t.Name == name {
				return p, &p.MCP.Tools[i]
			}
		}
	}
	return nil, nil
}

// callPluginTool runs the tool with its arguments as JSON on stdin and returns
// what it printed. A named site is handed over the way every surface does it,
// and the tool runs in that site's directory.
func callPluginTool(p *plugins.Plugin, t *plugins.MCPTool, args map[string]any) (any, *rpcError) {
	env := map[string]string{}
	dir := p.Dir
	if site, err := config.FindSite(strArg(args, "site")); err == nil {
		env = plugins.Env(plugins.SitePayload(site))
		dir = site.Path
	}
	cmd, err := p.Command(t.Run, plugins.SurfaceMCP, nil, env)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	stdin, _ := json.Marshal(args)
	var out bytes.Buffer
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := plugins.RunWithin(cmd, t.Limit()); err != nil {
		msg := strings.TrimSpace(out.String() + "\n" + err.Error())
		plugins.Failed(plugins.Failure{Plugin: p.Name, Surface: plugins.SurfaceMCP, What: t.Name, Error: msg})
		return toolErr(msg), nil
	}
	return toolOK(out.String()), nil
}
