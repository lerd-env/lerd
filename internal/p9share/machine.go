package p9share

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Mount is a 9p share as the machine config records it: the host folder, where
// the VM mounts it and the vsock port its client9p dials.
type Mount struct {
	Source string
	Target string
	Port   uint32
}

// ReadMachineMounts reads the 9p mounts from a podman machine's JSON config.
func ReadMachineMounts(path string) ([]Mount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Mounts []struct {
			Source      string
			Target      string
			Type        string
			VSockNumber *uint64
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var out []Mount
	for _, m := range cfg.Mounts {
		if m.Type != "9p" || m.VSockNumber == nil {
			continue
		}
		out = append(out, Mount{Source: m.Source, Target: m.Target, Port: uint32(*m.VSockNumber)})
	}
	return out, nil
}

// Remounts returns, in share order, the VM folder each share is mounted at. A
// share whose port the config does not record is refused rather than left
// unmounted.
func Remounts(shares []Share, mounts []Mount) ([]Mount, error) {
	byPort := make(map[uint32]Mount, len(mounts))
	for _, m := range mounts {
		byPort[m.Port] = m
	}
	out := make([]Mount, 0, len(shares))
	for _, s := range shares {
		port, err := Port(s.Service)
		if err != nil {
			return nil, err
		}
		m, ok := byPort[port]
		if !ok {
			return nil, fmt.Errorf("no machine mount uses vsock port %d (%s)", port, s.Dir)
		}
		out = append(out, m)
	}
	return out, nil
}

// UnmountScript detaches every mount in the VM. Lazy, so a process still
// holding a file there does not block the swap.
func UnmountScript(ms []Mount) string {
	targets := make([]string, len(ms))
	for i, m := range ms {
		targets[i] = shellQuote(m.Target)
	}
	return "sudo umount -l " + strings.Join(targets, " ") + " 2>/dev/null; true"
}

// MountScript mounts every share again through Podman's own client9p, which
// dials the port and mounts the folder the way the machine start did. A folder
// already mounted is skipped, so a retry after a partial failure is safe.
func MountScript(ms []Mount) string {
	cmds := make([]string, len(ms))
	for i, m := range ms {
		t := shellQuote(m.Target)
		cmds[i] = fmt.Sprintf("{ mountpoint -q %s || sudo podman machine client9p %d %s; }", t, m.Port, t)
	}
	return strings.Join(cmds, " && ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
