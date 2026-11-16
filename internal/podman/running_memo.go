package podman

import (
	"context"
	"strings"
	"sync"
	"time"
)

// runningMemoTTL covers the checks one command makes in a burst: a php run asks
// about every service its site uses, and each inspect is a VM round trip on macOS.
const runningMemoTTL = time.Second

var (
	runningMemoMu sync.Mutex
	runningMemo   map[string]string // running container name -> run id
	runningMemoAt time.Time
	// runningMemoListed is false when the ps behind the memo failed.
	runningMemoListed bool

	// runningPsFn lists lerd's running containers as "name\trunID" lines; a
	// seam for tests.
	runningPsFn = defaultRunningPs
)

func defaultRunningPs() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	defer cancel()
	out, err := CmdContext(ctx, "ps", "--filter", "name=lerd-",
		"--format", "{{.Names}}\t{{.StartedAt}}-{{.Pid}}").Output()
	return string(out), err
}

// memoRunID answers from one podman ps shared by every check in the next
// second. listed is false only when that ps failed, and callers then ask podman
// about the container directly; a name missing from a ps that worked is stopped.
func memoRunID(name string) (id string, running, listed bool) {
	runningMemoMu.Lock()
	defer runningMemoMu.Unlock()
	if runningMemo == nil || time.Since(runningMemoAt) > runningMemoTTL {
		runningMemo = map[string]string{}
		runningMemoAt = time.Now()
		out, err := runningPsFn()
		runningMemoListed = err == nil
		for _, line := range strings.Split(out, "\n") {
			if n, id, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok {
				runningMemo[n] = id
			}
		}
	}
	id, running = runningMemo[name]
	return id, running, runningMemoListed
}

func dropRunningMemo() {
	runningMemoMu.Lock()
	runningMemo = nil
	runningMemoMu.Unlock()
}

// ContainerRunID names the container's current run by its start time and pid,
// both new on every start; "" when it is not running.
func ContainerRunID(name string) string {
	if id, running, listed := memoRunID(name); running || listed {
		return id
	}
	out, err := Run("inspect", "--format={{.State.Running}} {{.State.StartedAt.Unix}}-{{.State.Pid}}", name)
	if err != nil {
		return ""
	}
	running, id, _ := strings.Cut(strings.TrimSpace(out), " ")
	if running != "true" {
		return ""
	}
	return id
}
