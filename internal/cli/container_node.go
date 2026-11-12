package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	nodeDet "github.com/geodro/lerd/internal/node"
	"github.com/geodro/lerd/internal/podman"
)

// containerNodeBin is the musl build of cwd's Node version when one is on
// disk, so node and npm in a PHP container match the Node lerd runs for the
// site on the host instead of whatever Alpine ships; "" when there is none.
func containerNodeBin(cwd string) string {
	if cwd == "" {
		return ""
	}
	spec, _ := nodeDet.DetectVersion(cwd)
	bin := nodeDet.MuslBinFor(shellWorkDir(cwd), spec)
	if !inContainer(bin) {
		return ""
	}
	return bin
}

// inContainer reports path inside the home folder, which the PHP containers
// mount at its own path, so they see it as it is.
func inContainer(path string) bool {
	home, err := os.UserHomeDir()
	return err == nil && path != "" && strings.HasPrefix(path, home+string(os.PathSeparator))
}

// withContainerNode puts cwd's Node in front of the image's PATH entries in
// path, which starts with podman.ContainerPath or carries it after a prefix.
func withContainerNode(cwd, path string) string {
	bin := containerNodeBin(cwd)
	if bin == "" {
		return path
	}
	return strings.Replace(path, podman.ContainerPath, bin+":"+podman.ContainerPath, 1)
}

// containerBindingsDeadline caps the fetch so a stalled registry delays a PHP
// command by seconds, not by download's minutes of retries.
const containerBindingsDeadline = 20 * time.Second

// ensureContainerBindings gives cwd's npm project the linux-musl builds Node in
// the PHP container loads. It only warns, under a short deadline: the PHP side
// of the run does not need them, and composer has to keep working offline.
func ensureContainerBindings(cwd string) {
	dir := nodeDet.NpmProjectDir(cwd)
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), containerBindingsDeadline)
	defer cancel()
	if err := nodeDet.FetchContainerBindings(ctx, dir, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "lerd: warning: Node in the PHP container may not load native packages: %v\n", err)
	}
}

// ensureContainerNode fetches the musl build of the exact version the host
// runs for cwd, once. lerd shell calls it: it is interactive, so the one-off
// download has someone to wait for it, and the exec shims never block on it.
func ensureContainerNode(cwd string, w io.Writer) {
	spec, _ := nodeDet.DetectVersion(cwd)
	mgr := nodeDet.Active()
	if spec == "" || !mgr.Available() {
		return
	}
	out, err := mgr.Command(spec, "node", []string{"-p", "process.version"}).Output()
	if err != nil {
		return // the host has no Node for this site either, so there is nothing to match
	}
	version := strings.TrimSpace(string(out))
	if !inContainer(nodeDet.MuslRoot()) {
		fmt.Fprintf(w, "  ! the shell keeps the image's own Node: %s is outside your home folder, which is all the container sees\n", nodeDet.MuslRoot())
		return
	}
	if _, err := os.Stat(filepath.Join(nodeDet.MuslRoot(), version, "bin", "node")); err != nil {
		fmt.Fprintf(w, "Fetching Node %s for the PHP container, once\n", version)
		if _, err := nodeDet.MuslBin(context.Background(), version, w); err != nil {
			fmt.Fprintf(w, "  ! the shell keeps the image's own Node: %v\n", err)
			return
		}
	}
	// The spec can name no version ("lts/iron") or match a newer cached patch
	// than the host now runs, so the shims follow what the host resolved.
	_ = nodeDet.RememberSpec(shellWorkDir(cwd), spec, version)
}
