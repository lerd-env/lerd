//go:build windows

package cli

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/geodro/lerd/internal/config"
)

// removeAfterExit hands path to a detached cmd that deletes it a few seconds
// after this process exits. Windows refuses to delete a running exe, and the
// uninstall runs from lerd.exe inside the very directory it is removing.
func removeAfterExit(path string) bool {
	if config.UnderTest() {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	script := deferredRemovalScript(path, info.IsDir())
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd.exe /c "` + script + `"`,
		HideWindow:    true,
		CreationFlags: 0x00000008 | syscall.CREATE_NEW_PROCESS_GROUP, // DETACHED_PROCESS
	}
	return cmd.Start() == nil
}

// deferredRemovalScript waits out lerd's exit, then deletes path.
func deferredRemovalScript(path string, dir bool) string {
	remove := `del /f /q "` + path + `"`
	if dir {
		remove = `rmdir /s /q "` + path + `"`
	}
	return strings.Join([]string{"ping -n 4 127.0.0.1 >nul", remove}, " & ")
}
