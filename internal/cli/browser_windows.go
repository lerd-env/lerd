//go:build windows

package cli

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows         = user32.NewProc("EnumWindows")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procKeybdEvent          = user32.NewProc("keybd_event")
)

// openBrowser hands the URL to the shell, which routes it to the default
// browser's running instance. rundll32 url.dll only did so reliably when no
// browser was open yet.
func openBrowser(url string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}

// openDashboard focuses a browser window already showing the dashboard rather
// than adding another tab. A window title only names its active tab, so a
// dashboard sitting in a background tab is not found. Otherwise it opens as a
// Chromium app window (the installed PWA when there is one), or a browser tab
// when no Chromium browser is installed.
func openDashboard(url string) error {
	if focusDashboardWindow() || openAppWindow(url) {
		return nil
	}
	return openBrowser(url)
}

func focusDashboardWindow() bool {
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if visible, _, _ := procIsWindowVisible.Call(hwnd); visible == 0 {
			return 1
		}
		buf := make([]uint16, 256)
		n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 0 && isDashboardWindowTitle(windows.UTF16ToString(buf[:n])) {
			found = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if found == 0 {
		return false
	}
	const swRestore = 9
	if iconic, _, _ := procIsIconic.Call(found); iconic != 0 {
		procShowWindow.Call(found, swRestore)
	}
	// Windows refuses focus to a background process. A synthetic Alt press marks
	// this one as having had input, which lets SetForegroundWindow through.
	const vkMenu, keyUp = 0x12, 0x2
	procKeybdEvent.Call(vkMenu, 0, 0, 0)
	procKeybdEvent.Call(vkMenu, 0, keyUp, 0)
	procShowWindow.Call(found, 5)
	procBringWindowToTop.Call(found)
	procSetForegroundWindow.Call(found)
	return true
}
