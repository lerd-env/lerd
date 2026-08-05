package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/geodro/lerd/internal/config"
)

// A tunnel is a child of lerd-ui, and killing lerd-ui is supposed to take it
// down with it: systemd kills the whole control group, and Pdeathsig covers a
// non-systemd run. macOS has neither. launchd only kills the job's own process
// group, and a tunnel is deliberately put in a group of its own so stopping it
// can take down the tool's children, so a killed lerd-ui leaves the tunnel
// running with the site still public and no dashboard entry left to stop it.
//
// Ordinary shutdowns are handled by the signal handler that calls
// StopAllTunnels. This file covers the rest: `launchctl kickstart -k` sends
// SIGKILL, which no handler can catch, so what is running is written down and
// the next lerd-ui start reaps whatever survived.

type tunnelRecord struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

var tunnelRecordMu sync.Mutex

// tunnelRecordPath is a var so tests can keep the record out of the real run dir.
var tunnelRecordPath = func() string {
	return filepath.Join(config.RunDir(), "tunnels.json")
}

// processCommand returns the full command line of a running pid, empty when it
// is gone. A var so tests can drive it without spawning processes.
var processCommand = func(pid int) string {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readTunnelRecords() []tunnelRecord {
	data, err := os.ReadFile(tunnelRecordPath())
	if err != nil {
		return nil
	}
	var recs []tunnelRecord
	if json.Unmarshal(data, &recs) != nil {
		return nil
	}
	return recs
}

func writeTunnelRecords(recs []tunnelRecord) {
	path := tunnelRecordPath()
	if len(recs) == 0 {
		os.Remove(path) //nolint:errcheck
		return
	}
	data, err := json.Marshal(recs)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0755) != nil {
		return
	}
	// 0600: the recorded command lines carry a Pinggy token as the SSH user,
	// the same secret the config file is kept private for.
	os.WriteFile(path, data, 0600) //nolint:errcheck
}

// recordTunnel notes a started tunnel, storing the command line so a later reap
// can tell the process apart from whatever else may hold that pid by then.
func recordTunnel(pid int, args []string) {
	tunnelRecordMu.Lock()
	defer tunnelRecordMu.Unlock()
	recs := append(readTunnelRecords(), tunnelRecord{PID: pid, Command: strings.Join(args, " ")})
	writeTunnelRecords(recs)
}

// forgetTunnel drops a tunnel that has been stopped or has exited.
func forgetTunnel(pid int) {
	tunnelRecordMu.Lock()
	defer tunnelRecordMu.Unlock()
	var kept []tunnelRecord
	for _, r := range readTunnelRecords() {
		if r.PID != pid {
			kept = append(kept, r)
		}
	}
	writeTunnelRecords(kept)
}

// tunnelKillFn signals a process, a seam so the reap can be tested without one.
var tunnelKillFn = func(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// ReapOrphanTunnels kills tunnels left behind by a previous lerd-ui, then
// clears the record. A pid is only signalled when the process still running
// under it is the one that was recorded, so a reused pid is left alone.
func ReapOrphanTunnels() {
	tunnelRecordMu.Lock()
	defer tunnelRecordMu.Unlock()
	for _, r := range readTunnelRecords() {
		if r.PID <= 0 || processCommand(r.PID) != r.Command {
			continue
		}
		// The tunnel leads its own process group, so the group takes the tool's
		// own children with it; the bare pid is the fallback for a tool that
		// somehow is not a group leader.
		if tunnelKillFn(-r.PID, syscall.SIGTERM) != nil {
			tunnelKillFn(r.PID, syscall.SIGTERM) //nolint:errcheck
		}
	}
	writeTunnelRecords(nil)
}
