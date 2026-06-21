package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewNodeUseCmd returns the node:use command.
func NewNodeUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node:use <version>",
		Short: "Set the default Node.js version",
		Args:  cobra.ExactArgs(1),
		RunE:  runNodeUse,
	}
}

func runNodeUse(_ *cobra.Command, args []string) error {
	if err := ensureNodeManaged(); err != nil {
		return err
	}
	major := strings.SplitN(args[0], ".", 2)[0]
	fnmPath := filepath.Join(config.BinDir(), "fnm")

	out, err := exec.Command(fnmPath, "default", major).CombinedOutput()
	if err != nil {
		return fmt.Errorf("fnm default %s: %s", major, strings.TrimSpace(string(out)))
	}

	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	cfg.Node.DefaultVersion = major
	if err := config.SaveGlobal(cfg); err != nil {
		return err
	}

	feedback.Begin()
	feedback.Done("default Node set to " + feedback.Val(major))
	return nil
}
