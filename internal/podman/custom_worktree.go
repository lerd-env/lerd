package podman

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// CustomWorktreeContainerName returns the Podman container name for a custom
// container site's git worktree, e.g. "lerd-custom-nestapp-feat-login". The
// branch is already sanitized by git.SanitizeBranch before it reaches here.
func CustomWorktreeContainerName(siteName, branch string) string {
	return CustomContainerName(siteName) + "-" + branch
}

// GenerateCustomWorktreeContainerQuadlet builds the quadlet for a worktree of
// a custom-container site. It mirrors the parent site's unit (same image,
// network, bind mounts and workdir) but has its own container name, so nginx
// can proxy the branch domain to the branch checkout while the parent keeps
// serving the main checkout.
//
// No PublishPort is emitted: nginx reaches the container by name on the lerd
// network, exactly as it does for the parent.
func GenerateCustomWorktreeContainerQuadlet(siteName, branch, projectPath string, port int) (string, error) {
	if !bindMountable(projectPath) {
		return "", fmt.Errorf("invalid worktree path %q for site %s: cannot be bind-mounted into a container", projectPath, siteName)
	}
	containerName := CustomWorktreeContainerName(siteName, branch)
	imageName := CustomImageName(siteName)

	var b strings.Builder

	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=Lerd custom worktree container (%s/%s)\n", siteName, branch)
	b.WriteString("After=network.target\n")

	b.WriteString("\n[Container]\n")
	fmt.Fprintf(&b, "Image=%s\n", imageName)
	fmt.Fprintf(&b, "ContainerName=%s\n", containerName)
	b.WriteString("Network=lerd\n")
	fmt.Fprintf(&b, "Volume=%s:/etc/hosts:ro,z\n", config.ContainerHostsFile())
	fmt.Fprintf(&b, "Volume=%s:%s:rw\n", projectPath, projectPath)
	fmt.Fprintf(&b, "Volume=%s:%s:ro,z\n", config.BinDir(), config.BinDir())
	fmt.Fprintf(&b, "Environment=LERD_SITE=%s\n", siteName)
	fmt.Fprintf(&b, "Environment=LERD_BRANCH=%s\n", branch)
	fmt.Fprintf(&b, "PodmanArgs=--security-opt=label=disable --workdir=%s\n", projectPath)

	b.WriteString("\n[Service]\n")
	b.WriteString("Restart=always\n")

	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=default.target\n")

	return b.String(), nil
}

// EnsureCustomWorktreeContainer writes and starts the worktree's quadlet. A
// worktree of a custom-container site is served by its own container so the
// branch keeps the same runtime as the parent (FrankenPHP, Node, Python, …)
// instead of falling back to the shared PHP-FPM pool.
func EnsureCustomWorktreeContainer(siteName, branch, projectPath string, port int) error {
	content, err := GenerateCustomWorktreeContainerQuadlet(siteName, branch, projectPath, port)
	if err != nil {
		return err
	}
	containerName := CustomWorktreeContainerName(siteName, branch)
	changed, err := WriteQuadletDiff(containerName, content)
	if err != nil {
		return err
	}
	if err := DaemonReloadIfNeeded(changed); err != nil {
		return err
	}
	ResetFailedUnit(containerName)
	return StartUnit(containerName)
}

// RemoveCustomWorktreeContainer stops and removes one worktree container and
// its quadlet. Missing units/containers are ignored.
func RemoveCustomWorktreeContainer(siteName, branch string) {
	name := CustomWorktreeContainerName(siteName, branch)
	_ = StopUnit(name)
	_ = RemoveQuadlet(name)
	RemoveContainer(name)
}

// RemoveAllCustomWorktreeContainers removes every worktree container belonging
// to a site. Called when the site itself is unlinked, where leaving branch
// containers behind would leak a runtime nothing can address.
func RemoveAllCustomWorktreeContainers(siteName string) {
	for _, name := range customWorktreeQuadletNames(siteName) {
		_ = StopUnit(name)
		_ = RemoveQuadlet(name)
		RemoveContainer(name)
	}
}

// CleanupStaleCustomWorktreeContainers removes worktree containers whose
// branch is no longer checked out. Without it a branch renamed or removed
// while lerd was offline would keep a container and a quadlet forever.
func CleanupStaleCustomWorktreeContainers(siteName string, liveBranches []string) {
	live := make(map[string]bool, len(liveBranches))
	for _, branch := range liveBranches {
		live[CustomWorktreeContainerName(siteName, branch)] = true
	}
	for _, name := range customWorktreeQuadletNames(siteName) {
		if live[name] {
			continue
		}
		_ = StopUnit(name)
		_ = RemoveQuadlet(name)
		RemoveContainer(name)
	}
}

// customWorktreeQuadletNames lists the worktree units for a site by their
// quadlet files. The parent's unit (lerd-custom-<site>.container) does not
// match the "<site>-" prefix and is left alone.
func customWorktreeQuadletNames(siteName string) []string {
	prefix := CustomContainerName(siteName) + "-"
	matches, err := filepath.Glob(filepath.Join(config.QuadletDir(), prefix+"*.container"))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		names = append(names, strings.TrimSuffix(base, ".container"))
	}
	return names
}

// CustomWorktreeQuadletExists reports whether a worktree unit file is present.
// Used by tests and by callers that need to distinguish "not created yet" from
// "created but not running".
func CustomWorktreeQuadletExists(siteName, branch string) bool {
	path := filepath.Join(config.QuadletDir(), CustomWorktreeContainerName(siteName, branch)+".container")
	_, err := os.Stat(path)
	return err == nil
}
