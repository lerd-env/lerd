//go:build darwin

package cli

import "os/exec"

func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}

// openDashboard is an ordinary browser tab on macOS; the app window is Linux's.
func openDashboard(url string) error {
	return openBrowser(url)
}
