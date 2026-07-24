package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSiteLinkArgs(t *testing.T) {
	// Without a requested name the positional is omitted, so a project's
	// committed .lerd.yaml domains are registered verbatim.
	got := siteLinkArgs("")
	if len(got) != 1 || got[0] != "link" {
		t.Errorf("siteLinkArgs(\"\") = %v, want [link]", got)
	}

	got = siteLinkArgs("myapp")
	if len(got) != 2 || got[0] != "link" || got[1] != "myapp" {
		t.Errorf("siteLinkArgs(\"myapp\") = %v, want [link myapp]", got)
	}
}

// The MCP server is launched by an editor whose PATH often lacks lerd's bin
// directory, so the subprocess must be the running binary, not a PATH lookup.
func TestLerdSelf_resolvesTheRunningBinary(t *testing.T) {
	self := lerdSelf()
	if self == "" {
		t.Fatal("lerdSelf() is empty")
	}
	if self == "lerd" {
		t.Skip("os.Executable unavailable on this platform; the fallback is all there is")
	}
	if !filepath.IsAbs(self) {
		t.Errorf("lerdSelf() = %q, want an absolute path", self)
	}
	if _, err := os.Stat(self); err != nil {
		t.Errorf("lerdSelf() = %q, which does not exist: %v", self, err)
	}
}

func TestExecSiteLink_rejectsAPathThatIsNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	out, rpcErr := execSiteLink(map[string]any{"path": file})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %v", rpcErr)
	}
	if !isToolError(out) {
		t.Errorf("linking a file must be an error, got %v", out)
	}
}

// isToolError reports whether a tool result carries the error flag.
func isToolError(out any) bool {
	m, ok := out.(map[string]any)
	if !ok {
		return false
	}
	v, _ := m["isError"].(bool)
	return v
}
