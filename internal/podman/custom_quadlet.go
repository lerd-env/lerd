package podman

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// GenerateCustomContainerQuadlet builds a quadlet .container file for a
// per-project custom container. The container joins the lerd network so it
// can reach services (lerd-mysql, lerd-redis, etc.) and is reachable by
// nginx via its container name.
//
// A project path that cannot be bind-mounted is refused rather than rendered
// without its Volume= line, which would leave the container with a --workdir
// pointing at nothing (#884).
func GenerateCustomContainerQuadlet(siteName, projectPath string, port int) (string, error) {
	if !bindMountable(projectPath) {
		return "", fmt.Errorf("invalid project path %q for site %s: cannot be bind-mounted into a container", projectPath, siteName)
	}
	containerName := CustomContainerName(siteName)
	imageName := CustomImageName(siteName)

	var b strings.Builder

	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=Lerd custom container (%s)\n", siteName)
	b.WriteString("After=network.target\n")

	b.WriteString("\n[Container]\n")
	fmt.Fprintf(&b, "Image=%s\n", imageName)
	fmt.Fprintf(&b, "ContainerName=%s\n", containerName)
	b.WriteString("Network=lerd\n")
	fmt.Fprintf(&b, "Volume=%s:/etc/hosts:ro,z\n", config.ContainerHostsFile())
	fmt.Fprintf(&b, "Volume=%s:%s:rw\n", projectPath, projectPath)
	fmt.Fprintf(&b, "PodmanArgs=--security-opt=label=disable --workdir=%s\n", projectPath)

	b.WriteString("\n[Service]\n")
	b.WriteString("Restart=always\n")

	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=default.target\n")

	return b.String(), nil
}

// WriteCustomContainerQuadlet writes the quadlet for a custom container site.
func WriteCustomContainerQuadlet(siteName, projectPath string, port int) error {
	content, err := GenerateCustomContainerQuadlet(siteName, projectPath, port)
	if err != nil {
		return err
	}
	_, err = WriteQuadletDiff(CustomContainerName(siteName), content)
	return err
}

// RemoveCustomContainerQuadlet removes the unit file for a custom container.
// RemoveQuadlet removes both the systemd .container file (Linux) and the
// launchd plist via RemoveContainerUnitFn (macOS), so this is now a thin
// wrapper kept for naming clarity at call sites.
func RemoveCustomContainerQuadlet(siteName string) error {
	return RemoveQuadlet(CustomContainerName(siteName))
}

// RemoveContainerUnitFn removes the platform-specific container unit file.
// On macOS this is set to services.Mgr.RemoveContainerUnit to remove the
// launchd plist. Nil on Linux (quadlet removal is sufficient).
var RemoveContainerUnitFn func(name string) error
