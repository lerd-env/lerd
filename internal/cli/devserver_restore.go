package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewDevServerFilesCmd returns the hidden `lerd dev-server-files` command a dev
// server's unit runs before every start.
func NewDevServerFilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "dev-server-files <path>",
		Short:  "Write back a checkout's generated dev server files (internal)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return RestoreDevServerFiles(args[0])
		},
	}
}

// RestoreDevServerFiles writes back the generated dev server files npm ci deletes
// with node_modules. The config is written whatever the workers declare, since a
// unit for one a branch change dropped still points at it.
func RestoreDevServerFiles(path string) error {
	site := siteOwning(path)
	if site == nil {
		return fmt.Errorf("no site owns %s", path)
	}
	tool := config.DevServerToolInstalled(path)
	if tool == nil {
		return nil
	}
	addr, err := devServerAddress(site, path)
	if err != nil {
		return err
	}
	_, err = writeDevServerWrapper(path, tool, addr)
	errs := []error{err}
	fw, ok := config.GetFrameworkForDir(site.Framework, path)
	if !ok || fw == nil {
		return errors.Join(errs...)
	}
	for name, w := range fw.Workers {
		command := resolveWorkerCommand(path, name, w)
		if !w.Host || config.DevServerToolForWorker(path, w, command) == nil || config.DevServerToolFor(path, command) != nil {
			continue
		}
		errs = append(errs, restoreDevServerValues(site, path, name, w, tool, addr))
	}
	return errors.Join(errs...)
}

// restoreDevServerValues writes the values module on the port the worker already
// pinned; one that never had a port has never run, and its first start writes it.
func restoreDevServerValues(site *config.Site, path, name string, w config.FrameworkWorker, tool *config.DevServerTool, addr devServerAddr) error {
	port := site.DevServerPort
	if !isSiteItself(site, path) {
		port = site.WorktreeDevPorts[filepath.Base(path)]
	}
	if w.Proxy.PinnedPort() {
		port = site.WorkerPorts[name]
	}
	if port == 0 {
		return nil
	}
	_, err := writeDevServerValues(path, tool, addr, port)
	return err
}

// devServerFilesStep is the shell command that writes a checkout's generated dev
// server files back, or empty when the checkout has no dev server installed.
func devServerFilesStep(sitePath string) string {
	if config.DevServerToolInstalled(sitePath) == nil {
		return ""
	}
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	return podman.ShellQuote(self) + " dev-server-files " + podman.ShellQuote(sitePath)
}

// siteOwning returns the site at path, or the site a worktree at path belongs to.
func siteOwning(path string) *config.Site {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(abs); err != nil {
		return nil
	}
	if s, err := config.FindSiteByPath(abs); err == nil && s != nil {
		return s
	}
	if s, ok := config.ParentSiteForWorktreeDir(abs); ok {
		return s
	}
	return nil
}
