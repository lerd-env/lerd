package mcp

import (
	"strings"
	"testing"
)

// A client that closes stdin while a call runs must end the session at once,
// not after the call returns, so the call's context is what EOF cancels.
func TestReadRequests_cancelsTheSessionAtEOF(t *testing.T) {
	cancelled := false
	lines, scanErr := readRequests(strings.NewReader("{\"id\":1}\n{\"id\":2}\n"), func() { cancelled = true })
	var got []string
	for l := range lines {
		got = append(got, string(l))
	}
	if strings.Join(got, "|") != `{"id":1}|{"id":2}` {
		t.Fatalf("lines = %q", got)
	}
	if !cancelled {
		t.Fatal("EOF did not cancel the session")
	}
	if *scanErr != nil {
		t.Fatalf("scan error = %v", *scanErr)
	}
}
