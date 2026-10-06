package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/editor"
	"github.com/geodro/lerd/internal/siteops"
	"github.com/spf13/cobra"
)

// NewCodeCmd returns the code command.
func NewCodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "code [site]",
		Short: "Open the site in the configured editor",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runCode,
	}
}

// codeTargetDir resolves the directory to open, the same way `lerd open`
// resolves the URL: a name comes from the registry, and with no argument the
// current directory decides. A worktree opens itself, since it is a checkout of
// its own rather than the parent whose registration it inherits.
func codeTargetDir(args []string) (string, error) {
	if len(args) > 0 {
		site, err := config.FindSite(args[0])
		if err != nil {
			return "", fmt.Errorf("site %q not found", args[0])
		}
		return site.Path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if site, err := config.FindSiteByPath(cwd); err == nil && site != nil {
		return site.Path, nil
	}
	if _, _, ok := findOwningWorktree(cwd); ok {
		return cwd, nil
	}
	// Fall back: maybe cwd is named like a site.
	name, _ := siteops.SiteNameAndDomain(filepath.Base(cwd), "test")
	if site, err := config.FindSite(name); err == nil {
		return site.Path, nil
	}
	site, err := ensureSiteForCwd()
	if err != nil {
		return "", err
	}
	return site.Path, nil
}

func runCode(_ *cobra.Command, args []string) error {
	dir, err := codeTargetDir(args)
	if err != nil {
		return err
	}
	argv := editor.DirCommand(dir)
	if len(argv) == 0 {
		return fmt.Errorf("no editor found; set `editor` in ~/.config/lerd/config.yaml")
	}
	fmt.Printf("Opening %s\n", dir)
	if err := startEditorDetached(argv); err != nil {
		return fmt.Errorf("launching editor: %w", err)
	}
	return nil
}

// startEditorDetached starts the editor and returns, leaving it running.
func startEditorDetached(argv []string) error {
	null, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer null.Close()
	return editorCmd(argv, null).Start()
}

// editorCmd builds the editor process. It runs in a session of its own, so it
// outlives the lerd process that spawned it and the terminal that ran it:
// started as a plain child it sits in the shell's process group, and closing the
// terminal takes the editor down with it. Its stdio goes to null, since the
// editor is a window and a GUI launcher's startup chatter has no business on the
// shell it was run from.
func editorCmd(argv []string, null *os.File) *exec.Cmd {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}
