package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// redirectStderr sends this process's stderr, Go's and the Win32 handle, to f.
func redirectStderr(f *os.File) {
	os.Stderr = f
	windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd())) //nolint:errcheck
}

// claimAgent takes the agent's named mutex for the rest of the process, and
// reports false when another agent in this session holds it. The desktop app
// starts the agent at every login and again whenever it exits, so a second
// one must step aside rather than fight the first for port 53. An agent
// started elevated, by the installer, creates the mutex with a DACL an
// unelevated one cannot open, which also means it is taken.
func claimAgent() bool {
	return claimMutex(`Local\lerd-agent`)
}

func claimMutex(name string) bool {
	_, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(name))
	return err != windows.ERROR_ALREADY_EXISTS && err != windows.ERROR_ACCESS_DENIED
}

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
