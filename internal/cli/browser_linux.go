//go:build linux

package cli

import (
	"errors"

	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/wsl"
)

func openBrowser(url string) error {
	if wsl.IsWSL() {
		return wsl.Open(url)
	}
	cmd := platform.OpenCommand(url)
	if cmd == nil {
		return errors.New("no browser opener found; install xdg-utils")
	}
	return cmd.Start()
}
