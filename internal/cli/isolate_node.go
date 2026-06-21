package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewIsolateNodeCmd returns the isolate:node command.
func NewIsolateNodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "isolate:node <version>",
		Short: "Pin the Node.js version for the current directory",
		Args:  cobra.ExactArgs(1),
		RunE:  runIsolateNode,
	}
}

func runIsolateNode(_ *cobra.Command, args []string) error {
	version := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	nodeVersionFile := filepath.Join(cwd, ".node-version")
	if err := os.WriteFile(nodeVersionFile, []byte(version+"\n"), 0644); err != nil {
		return fmt.Errorf("writing .node-version: %w", err)
	}

	// Persist node_version to .lerd.yaml so the override travels with the
	// branch (worktree) or with the project (parent site). For worktrees the
	// file is created if missing; for parents we only touch an existing file.
	if _, _, ok := FindParentSiteForWorktree(cwd); ok {
		if err := config.SetWorktreeNodeVersion(cwd, version); err != nil {
			feedback.Warn("updating .lerd.yaml: %v", err)
		}
	} else {
		_ = updateProjectNodeVersionIfExists(cwd, version)
	}

	feedback.Begin()
	feedback.Done("Node pinned to " + feedback.Val(version))

	// Run fnm install for this version
	fnmPath := filepath.Join(config.BinDir(), "fnm")
	if _, err := os.Stat(fnmPath); err == nil {
		cmd := exec.Command(fnmPath, "install", version)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			feedback.Warn("fnm install %s: %v", version, err)
		}
	} else {
		feedback.Warn("fnm not found — run 'lerd install' to set up Node.js management")
	}

	return nil
}

// updateProjectNodeVersionIfExists writes node_version to .lerd.yaml only when
// the file is already present. Mirrors the no-op-on-missing semantics of
// SetProjectPHPVersion so plain `lerd isolate:node` runs on parent sites that
// haven't opted into .lerd.yaml stay quiet.
func updateProjectNodeVersionIfExists(dir, version string) error {
	path := filepath.Join(dir, ".lerd.yaml")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	cfg, err := config.LoadProjectConfig(dir)
	if err != nil || cfg == nil {
		return err
	}
	cfg.NodeVersion = version
	return config.SaveProjectConfig(dir, cfg)
}
