package podman

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/platform"
	"github.com/geodro/lerd/internal/portforward"
)

// LoopbackPortsPublishedWide reports, per installed lerd container, the host
// ports its unit binds to loopback that the platform publishes on every
// interface instead. Empty wherever a loopback bind holds, including ports
// the pf redirect keeps on loopback.
func LoopbackPortsPublishedWide() (map[string][]int, error) {
	wide := map[string][]int{}
	if !platform.Current.PrivilegedPortsBindAll {
		return wide, nil
	}
	paths, err := filepath.Glob(filepath.Join(config.QuadletDir(), "lerd-*.container"))
	if err != nil {
		return nil, err
	}
	redirected := portforward.Redirected()
	for _, path := range paths {
		ports, err := widePortsIn(path, redirected)
		if err != nil {
			return nil, err
		}
		if len(ports) > 0 {
			wide[strings.TrimSuffix(filepath.Base(path), ".container")] = ports
		}
	}
	return wide, nil
}

// UnitLoopbackPortsPublishedWide is LoopbackPortsPublishedWide for one unit,
// reading only its file; a unit that is not installed publishes nothing.
func UnitLoopbackPortsPublishedWide(unit string) ([]int, error) {
	if !platform.Current.PrivilegedPortsBindAll {
		return nil, nil
	}
	ports, err := widePortsIn(filepath.Join(config.QuadletDir(), unit+".container"), portforward.Redirected())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return ports, err
}

// widePortsIn lists the loopback ports below 1024 the unit at path publishes
// that no loopback forward covers.
func widePortsIn(path string, redirected map[int]bool) ([]int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ports []int
	for _, p := range privilegedLoopbackPorts(string(content)) {
		if !redirected[p] {
			ports = append(ports, p)
		}
	}
	return ports, nil
}

// privilegedLoopbackPorts lists, once each and in file order, the host ports
// below 1024 that content publishes on 127.0.0.1 or [::1].
func privilegedLoopbackPorts(content string) []int {
	var ports []int
	seen := map[int]bool{}
	for _, line := range strings.Split(content, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "PublishPort=")
		if !ok {
			continue
		}
		rest, ok := strings.CutPrefix(value, "127.0.0.1:")
		if !ok {
			if rest, ok = strings.CutPrefix(value, "[::1]:"); !ok {
				continue
			}
		}
		hostPort, _, _ := strings.Cut(rest, ":")
		n, err := strconv.Atoi(hostPort)
		if err != nil || n <= 0 || n >= 1024 || seen[n] {
			continue
		}
		seen[n] = true
		ports = append(ports, n)
	}
	return ports
}
