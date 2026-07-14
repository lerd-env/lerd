package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// requiredWorktreeDBChoice returns the database choice a framework definition
// forces for a new worktree, or "" when the user should be prompted as usual.
// An app that keeps deployment state in the database cannot share the parent's:
// applying the worktree's own config there would break the parent site.
func requiredWorktreeDBChoice(fw *config.Framework) string {
	if fw == nil || fw.Worktree == nil || !strings.EqualFold(fw.Worktree.DBIsolation, "required") {
		return ""
	}
	if strings.EqualFold(fw.Worktree.DBSource, "main") {
		return "clone-main"
	}
	return "empty"
}

// worktreeSetupArgs turns a command the definition declares into the console
// invocation that runs it, so nothing in Go needs to know what the command means.
// Returns nil when the framework declares no console to run it with.
func worktreeSetupArgs(fw *config.Framework, command string) []string {
	if fw == nil || fw.Console == "" || strings.TrimSpace(command) == "" {
		return nil
	}
	return append([]string{fw.Console}, strings.Fields(command)...)
}

// runWorktreeSetupCommands runs the console commands a framework declares for a
// new worktree, once its env file and database are both in place. Magento seeds
// its own base URL into env.php, which changes the config hash it stores in the
// database, and it refuses to serve until that config is imported.
func runWorktreeSetupCommands(fw *config.Framework, worktreePath string, log io.Writer) {
	if fw == nil || fw.Worktree == nil {
		return
	}
	for _, command := range fw.Worktree.Commands {
		args := worktreeSetupArgs(fw, command)
		if args == nil {
			continue
		}
		step := feedback.StartOn(log, command)
		code, err := RunPHPCapture(worktreePath, args)
		if err == nil && code != 0 {
			err = fmt.Errorf("exited %d", code)
		}
		if err != nil {
			step.Fail(err)
			continue
		}
		step.OK("")
	}
}
