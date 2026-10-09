//go:build windows

package hostshell

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

// createNoWindow keeps a console program started by a process with no console
// of its own, such as lerd-ui, from flashing a window up while it runs.
const createNoWindow = 0x08000000

// Command runs shell through PowerShell. The trailer makes PowerShell exit
// with the command's own code, which it otherwise flattens to 0 or 1, and with
// 1 when a cmdlet or a missing program failed.
func Command(ctx context.Context, shell string) *exec.Cmd {
	return exec.CommandContext(ctx, Bin(), "-NoLogo", "-NoProfile", "-NonInteractive",
		"-EncodedCommand", Encode(script(shell)))
}

func script(shell string) string {
	return shell + "\nif ($?) { exit 0 }\nif ($LASTEXITCODE) { exit $LASTEXITCODE }\nexit 1"
}

// Hide starts cmd without a console window. Only callers with no console of
// their own want it: a CLI command would lose the terminal it reads input from.
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// Bin prefers PowerShell 7, which understands the && and || that command
// lines written for sh chain steps with, over Windows PowerShell 5.1. A PATH
// without either still finds 5.1 where Windows always installs it.
func Bin() string {
	for _, name := range []string{"pwsh", "powershell"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

// Quote single-quotes s for PowerShell, where only a doubled quote is special.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Encode is the base64 of the UTF-16LE text -EncodedCommand expects. Passing a
// script encoded keeps wt.exe, which splits its command line on ';', and
// cmd.exe from reinterpreting it.
func Encode(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
