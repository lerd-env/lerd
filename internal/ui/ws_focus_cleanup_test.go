package ui

import (
	"os"
	"strings"
	"testing"
)

// The focus/visibility decrement in handleWS reads connFocused/connVisible,
// which only the reader goroutine writes. If the cleanup runs while the reader
// is still live, a lost focus decrement leaves focusedClients above zero for the
// life of the process and silences every native notification and web push. The
// cleanup therefore has to close the socket (to unblock ReadFrame) and join the
// reader (<-done) before it reads the flags. This pins that ordering so a
// refactor that reintroduces the race fails here rather than in the field.
func TestHandleWS_cleanupJoinsReaderBeforeReadingFocusFlags(t *testing.T) {
	src, err := os.ReadFile("ws.go")
	if err != nil {
		t.Fatalf("reading ws.go: %v", err)
	}
	body := string(src)

	closeAt := strings.Index(body, "ws.Close()")
	// The join after the close, so the match is the code and not the comment
	// above the cleanup that mentions the reader's done channel in prose.
	joinAt := -1
	if closeAt >= 0 {
		if rel := strings.Index(body[closeAt:], "<-done"); rel >= 0 {
			joinAt = closeAt + rel
		}
	}
	focusAt := strings.Index(body, "noteFocus(false)")
	visibleAt := strings.Index(body, "noteVisibility(false)")

	if closeAt < 0 || joinAt < 0 || focusAt < 0 || visibleAt < 0 {
		t.Fatal("handleWS cleanup must close the socket, join the reader, then decrement focus and visibility")
	}
	if !(closeAt < joinAt && joinAt < focusAt && joinAt < visibleAt) {
		t.Errorf("cleanup order is wrong: close=%d join=%d focus=%d visible=%d; the join must sit after the close and before both decrements",
			closeAt, joinAt, focusAt, visibleAt)
	}
	// A standalone `defer ws.Close()` up top would run last and defeat the join,
	// so the socket close must live inside the coordinated cleanup, not on its own.
	if strings.Contains(body, "\tdefer ws.Close()") {
		t.Error("ws.Close must run inside the reader-joining cleanup, not as a standalone top-level defer")
	}
}
