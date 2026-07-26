package cli

import (
	"strings"
	"testing"
)

// The no-prompter branch is reached exactly when the caller has no terminal, so
// it must not emit styling. lipgloss colours its output even when stdout is a
// pipe, unlike the feedback layer, and the escape codes ended up verbatim in the
// MCP tool result and the dashboard's link log.
func TestConfirmReplace_withoutAPrompterEmitsNoEscapeCodes(t *testing.T) {
	out := captureStdout(t, func() {
		if _, err := confirmReplaceWith(nil, "framework", "laravel", def{Label: "A"}, def{Label: "B"}); err != nil {
			t.Error(err)
		}
	})

	if strings.Contains(out, "\x1b") {
		t.Errorf("output carries ANSI escape codes: %q", out)
	}
	if !strings.Contains(out, "framework/laravel differs") {
		t.Errorf("the conflict was not reported: %q", out)
	}
}
