package mcp

import (
	"slices"
	"strings"
	"testing"
)

func TestExecOffersPunAndNeedsArgs(t *testing.T) {
	if !slices.Contains(execTool().InputSchema.Properties["action"].Enum, "pun") {
		t.Error("exec does not offer the pun action")
	}
	got, _ := execPun(map[string]any{"path": t.TempDir()})
	if !strings.Contains(toolText(got), "args is required") {
		t.Errorf("got %q, want the args error", toolText(got))
	}
}
