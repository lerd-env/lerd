//go:build !nogui && !windows

package tray

import (
	"fmt"
	"os/exec"
)

func openUpdateTerminal(latestVer string) {
	script := fmt.Sprintf(
		`echo "Lerd update available: v%s"; `+
			`read -rp "Update now? [y/N] " ans; `+
			`[[ "$ans" =~ ^[Yy]$ ]] && lerd update; `+
			`echo; read -rp "Press Enter to close..."`,
		latestVer,
	)
	terminals := [][]string{
		{"konsole", "-e", "bash", "-c", script},
		{"ptyxis", "--", "bash", "-c", script},
		{"gnome-terminal", "--", "bash", "-c", script},
		{"xfce4-terminal", "-e", "bash -c '" + script + "'"},
		{"xterm", "-e", "bash", "-c", script},
	}
	for _, t := range terminals {
		if _, err := exec.LookPath(t[0]); err == nil {
			_ = exec.Command(t[0], t[1:]...).Start()
			return
		}
	}
}
