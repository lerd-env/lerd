package main

import "golang.org/x/sys/windows"

// disableQuickEdit stops a click inside the installer's console from pausing
// it: in QuickEdit mode Windows freezes any program that writes to a console
// while text is being selected, which looks exactly like a hung install.
func disableQuickEdit() {
	h, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return
	}
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return
	}
	windows.SetConsoleMode(h, (mode&^windows.ENABLE_QUICK_EDIT_MODE)|windows.ENABLE_EXTENDED_FLAGS) //nolint:errcheck
}
