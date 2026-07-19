package mcp

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/phpini"
)

// A read against the shared scope must resolve without a version and hand back
// the seeded template when no override exists yet.
func TestExecPHPIniRead_SharedScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	res, rpcErr := execPHPIniRead(map[string]any{"shared": true})
	if rpcErr != nil {
		t.Fatalf("unexpected rpcError: %v", rpcErr)
	}
	if mcpIsError(res) {
		t.Fatalf("shared read errored: %v", res)
	}
	msg := mcpText(t, res)
	if !strings.Contains(msg, phpini.SharedScope) {
		t.Errorf("read output %q should name the shared scope", msg)
	}
	if !strings.Contains(msg, "applied to every PHP version") {
		t.Errorf("read output should include the shared template guidance:\n%s", msg)
	}
}

// An unknown version scope is rejected before any disk write.
func TestExecPHPIniWrite_RejectsUnknownScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	res, rpcErr := execPHPIniWrite(map[string]any{"version": "9.9", "content": "memory_limit = 1G\n"})
	if rpcErr != nil {
		t.Fatalf("unexpected rpcError: %v", rpcErr)
	}
	if !mcpIsError(res) {
		t.Fatalf("expected an error result for an unknown version scope, got %v", res)
	}
	if msg := mcpText(t, res); !strings.Contains(msg, "invalid php.ini scope") {
		t.Errorf("error message = %q, want it to flag the invalid scope", msg)
	}
}
