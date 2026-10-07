package ui

import (
	"strings"
	"testing"
)

// The dashboard's container-shell and follow-logs actions run a script in a
// terminal, which is a different code path from opening a directory. It has to
// honour the same choice, or picking a terminal changes one button and not the
// other.
func TestTerminalScriptCandidates_putsTheChosenTerminalFirst(t *testing.T) {
	t.Setenv("TERMINAL", "my-terminal")
	got := terminalScriptCandidates("echo hi")
	if len(got) == 0 || got[0].bin != "my-terminal" {
		t.Fatalf("first candidate = %+v, want my-terminal", got)
	}
}

// kitty takes the program directly and rejects `-e`, so a chosen kitty launched
// with the generic form opens nothing at all.
func TestNamedTerminalCommand_reusesTheFlagsLerdKnows(t *testing.T) {
	got := namedTerminalCommand("kitty", "echo hi")
	if got.bin != "kitty" || strings.Join(got.args, " ") != "sh -c echo hi" {
		t.Errorf("namedTerminalCommand(kitty) = %+v, want kitty sh -c echo hi", got)
	}

	// An alternatives link resolves to an absolute path and still names the
	// emulator it points at.
	if got := namedTerminalCommand("/usr/bin/ghostty", "echo hi"); got.bin != "/usr/bin/ghostty" || got.args[0] != "-e" || len(got.args) != 2 {
		t.Errorf("namedTerminalCommand(/usr/bin/ghostty) = %+v, want its own single-argument form", got)
	}
}

func TestNamedTerminalCommand_fallsBackToTheGenericForm(t *testing.T) {
	got := namedTerminalCommand("some-terminal", "echo hi")
	if got.bin != "some-terminal" {
		t.Errorf("bin = %q, want some-terminal", got.bin)
	}
	if strings.Join(got.args, " ") != "-e sh -c echo hi" {
		t.Errorf("args = %v, want the generic -e form", got.args)
	}
}

// The desktop's own choice outranks whatever happens to be on PATH, the same
// way it does when opening a directory.
func TestTerminalScriptCandidates_honoursTheSystemDefault(t *testing.T) {
	t.Setenv("TERMINAL", "")
	prev := defaultTerminal
	defaultTerminal = func() string { return "foot" }
	t.Cleanup(func() { defaultTerminal = prev })

	got := terminalScriptCandidates("echo hi")
	if len(got) == 0 || got[0].bin != "foot" {
		t.Fatalf("first candidate = %+v, want foot", got)
	}
	if strings.Join(got[0].args, " ") != "sh -c echo hi" {
		t.Errorf("args = %v, want foot's own form", got[0].args)
	}
}
