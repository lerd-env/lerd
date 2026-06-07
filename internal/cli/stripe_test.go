package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func findCmd(cmds []*cobra.Command, use string) *cobra.Command {
	for _, c := range cmds {
		if c.Use == use {
			return c
		}
	}
	return nil
}

func TestNewStripeCmds_ConfigCommandAndFlags(t *testing.T) {
	cmds := NewStripeCmds()

	cfg := findCmd(cmds, "stripe:config")
	if cfg == nil {
		t.Fatal("NewStripeCmds() must include a stripe:config command")
	}
	for _, f := range []string{"path", "secret-env-key"} {
		if cfg.Flags().Lookup(f) == nil {
			t.Errorf("stripe:config missing --%s flag", f)
		}
	}

	// The listener command must also expose the persistence flags so the path
	// can be set inline when starting.
	listen := findCmd(cmds, "stripe:listen")
	if listen == nil {
		t.Fatal("NewStripeCmds() must include a stripe:listen command")
	}
	for _, f := range []string{"path", "secret-env-key", "api-key"} {
		if listen.Flags().Lookup(f) == nil {
			t.Errorf("stripe:listen missing --%s flag", f)
		}
	}
}
