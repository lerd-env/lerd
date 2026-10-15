package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/plugins"
)

// installEchoPlugin installs a plugin "echo" with one tool that prints its
// surface, its stdin and an exit code taken from the arguments.
func installEchoPlugin(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := filepath.Join(plugins.Dir(), "echo")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `api_version: 1
name: echo
mcp:
  tools:
    - name: say
      description: Echo the arguments back
      input_schema:
        type: object
        properties:
          fail: {type: string}
      run: bin/say
`
	script := "#!/bin/sh\nin=$(cat)\necho \"$LERD_SURFACE $in\"\ncase \"$in\" in *'\"fail\":\"yes\"'*) exit 4;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "say"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPluginTools_ListedBesideBuiltins(t *testing.T) {
	installEchoPlugin(t)
	res, rpcErr := dispatch(&rpcRequest{Method: "tools/list"})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	raw, _ := json.Marshal(res)
	var out struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range out.Tools {
		names = append(names, tool.Name)
		if tool.Name == "echo_say" && tool.InputSchema["type"] != "object" {
			t.Errorf("plugin schema not passed through: %v", tool.InputSchema)
		}
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "echo_say") || !strings.Contains(joined, "site") {
		t.Errorf("want built-ins and echo_say, got %s", joined)
	}
}

func TestPluginTools_CallRunsToolWithArgsOnStdin(t *testing.T) {
	installEchoPlugin(t)
	res, rpcErr := handleToolCall(json.RawMessage(`{"name":"echo_say","arguments":{"fail":"no"}}`))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	text := resultText(t, res)
	if !strings.Contains(text, `mcp {"fail":"no"}`) {
		t.Errorf("tool output = %q", text)
	}
	if resultIsError(res) {
		t.Error("a zero exit is not an error")
	}
}

func TestPluginTools_NonZeroExitIsToolError(t *testing.T) {
	installEchoPlugin(t)
	res, rpcErr := handleToolCall(json.RawMessage(`{"name":"echo_say","arguments":{"fail":"yes"}}`))
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	if !resultIsError(res) {
		t.Errorf("a failing tool must come back as isError, got %v", res)
	}
}
