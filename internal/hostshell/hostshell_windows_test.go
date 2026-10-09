//go:build windows

package hostshell

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Site commands ran through sh, which Windows does not have, so every one
// failed to start. They now run in PowerShell and exit with the command's own
// code.
func TestCommandRunsPowerShellWithTheCommandsExitCode(t *testing.T) {
	cmd := Command(context.Background(), `cmd /c "exit 3"`)
	if base := strings.ToLower(filepath.Base(cmd.Path)); !strings.HasPrefix(base, "pwsh") && !strings.HasPrefix(base, "powershell") {
		t.Fatalf("runs %s, want PowerShell", cmd.Path)
	}
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("err = %v, want exit status 3", err)
	}
	for shell, want := range map[string]int{`cmd /c "exit 0"`: 0, "Write-Output hi": 0, "lerd-no-such-program": 1} {
		code := 0
		if err := Command(context.Background(), shell).Run(); errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if code != want {
			t.Errorf("%q exited %d, want %d", shell, code, want)
		}
	}
}

func TestHideKeepsFlagsAlreadySet(t *testing.T) {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = nil
	Hide(cmd)
	cmd.SysProcAttr.CreationFlags |= 0x200
	Hide(cmd)
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags != createNoWindow|0x200 {
		t.Errorf("SysProcAttr = %+v, want hidden with both flags", cmd.SysProcAttr)
	}
}
