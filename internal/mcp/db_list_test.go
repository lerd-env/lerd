package mcp

import (
	"strings"
	"testing"
)

// TestExecDBList_noIntrospectCommand asserts an engine whose preset declares no
// introspection reports that rather than erroring or reaching for a container.
func TestExecDBList_noIntrospectCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	res, rpcErr := execDBList(map[string]any{"service": "nosuchengine"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "no database introspection") {
		t.Errorf("got %q, want a no-introspection report", text)
	}
}

// TestExecDBList_requiresATarget asserts list refuses to guess when neither a
// service nor a resolvable project is given.
func TestExecDBList_requiresATarget(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	res, rpcErr := execDBList(map[string]any{"path": t.TempDir()})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if text := resultText(t, res); !strings.Contains(text, "service") {
		t.Errorf("got %q, want guidance to pass a service", text)
	}
}
