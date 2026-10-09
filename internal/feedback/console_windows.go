package feedback

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableANSI turns on virtual terminal processing so the legacy console
// (conhost, used by Windows PowerShell 5.1) renders ANSI codes instead of
// printing them raw. A console that refuses it gets plain output.
func enableANSI() bool {
	enableVirtualTerminal(os.Stderr)
	return enableVirtualTerminal(os.Stdout)
}

// enableVirtualTerminal turns on ANSI escape handling for f's console and
// reports whether it is now active; false for anything that isn't a console.
func enableVirtualTerminal(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
