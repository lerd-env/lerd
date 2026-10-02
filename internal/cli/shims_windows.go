//go:build windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/geodro/lerd/internal/config"
)

// cmdShim is the Windows twin of an sh shim: PowerShell and cmd only resolve
// names with a PATHEXT extension, so `php` has to exist as php.cmd. It runs the
// recorded lerd binary, or the installed one when that path has gone.
func cmdShim(lerdBin, fallback, command string) string {
	return "@echo off\r\n" +
		"setlocal\r\n" +
		fmt.Sprintf("set \"LERD=%s\"\r\n", lerdBin) +
		fmt.Sprintf("if not exist \"%%LERD%%\" set \"LERD=%s\"\r\n", fallback) +
		fmt.Sprintf("\"%%LERD%%\" %s %%*\r\n", command) +
		"exit /b %ERRORLEVEL%\r\n"
}

// writeCmdShims writes <name>.cmd for every entry, each running the lerd
// subcommand it maps to; an empty command removes that shim instead.
func writeCmdShims(binDir, lerdBin string, shims map[string]string) error {
	fallback := filepath.Join(config.BinDir(), "lerd.exe")
	for name, command := range shims {
		path := filepath.Join(binDir, name+".cmd")
		if command == "" {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing %s shim: %w", name, err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte(cmdShim(lerdBin, fallback, command)), 0o755); err != nil {
			return fmt.Errorf("writing %s shim: %w", name, err)
		}
	}
	return nil
}

// writeUserPathEntry puts dir on the user's PATH in the registry, where every
// new terminal and app picks it up. Windows has no shell rc file to append to.
func writeUserPathEntry(dir string) (bool, error) {
	return true, updateUserPath(func(p string) string { return withPathEntry(p, dir) })
}

// removeUserPathEntry takes dir back off the user's PATH.
func removeUserPathEntry(dir string) bool {
	_ = updateUserPath(func(p string) string { return withoutPathEntry(p, dir) })
	return true
}

func updateUserPath(edit func(string) string) error {
	// The user's registry hive has no per-test copy, so a test must never reach it.
	if config.UnderTest() {
		return nil
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening the user environment: %w", err)
	}
	defer k.Close()
	cur, valType, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("reading the user PATH: %w", err)
	}
	next := edit(cur)
	if next == cur {
		return nil
	}
	if valType == registry.SZ {
		err = k.SetStringValue("Path", next)
	} else {
		err = k.SetExpandStringValue("Path", next)
	}
	if err != nil {
		return fmt.Errorf("writing the user PATH: %w", err)
	}
	broadcastEnvironmentChange()
	return nil
}

// withPathEntry prepends dir to a ;-separated PATH unless it is already on it.
func withPathEntry(pathList, dir string) string {
	parts := splitPathList(pathList)
	for _, p := range parts {
		if samePathEntry(p, dir) {
			return strings.Join(parts, ";")
		}
	}
	return strings.Join(append([]string{dir}, parts...), ";")
}

// withoutPathEntry drops every spelling of dir from a ;-separated PATH.
func withoutPathEntry(pathList, dir string) string {
	var kept []string
	for _, p := range splitPathList(pathList) {
		if !samePathEntry(p, dir) {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ";")
}

func splitPathList(pathList string) []string {
	var parts []string
	for _, p := range strings.Split(pathList, ";") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func samePathEntry(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, `\/`), strings.TrimRight(b, `\/`))
}

// broadcastEnvironmentChange tells Explorer the environment changed, so
// terminals opened from it afterwards see the new PATH without a sign-out.
func broadcastEnvironmentChange() {
	env, _ := syscall.UTF16PtrFromString("Environment")
	user32 := windows.NewLazySystemDLL("user32.dll")
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	user32.NewProc("SendMessageTimeoutW").Call( //nolint:errcheck
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, 0)
}
