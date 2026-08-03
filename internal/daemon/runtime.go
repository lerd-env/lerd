// Package daemon holds process-level setup shared by lerd's long-running
// processes: lerd-ui, the watcher, the tray, and the MCP server.
package daemon

import (
	"os"
	"runtime"
)

// maxProcs caps the Go scheduler's parallelism for a background daemon. Four
// leaves room for the handful of goroutines that do run together (a snapshot
// rebuild while a request is served) without sizing the runtime to the machine.
const maxProcs = 4

// targetProcs is the effective GOMAXPROCS for a machine with numCPU cores. It
// only ever lowers: a laptop with fewer cores than the cap keeps what it has.
func targetProcs(numCPU int) int {
	if numCPU < 1 {
		return 1
	}
	if numCPU > maxProcs {
		return maxProcs
	}
	return numCPU
}

// TuneRuntime caps GOMAXPROCS and reports the value in effect.
//
// lerd's daemons are event handlers, not parallel compute: they sit on sockets,
// timers and subprocesses. Left at the default, Go sizes the scheduler to every
// core, so on a many-core machine each daemon carries dozens of threads and a
// single wakeup fans out into futex traffic across them. Capping keeps that
// fan-out small, which is what a laptop feels. Goroutines blocked in syscalls
// still get threads of their own, so subprocess and file concurrency is
// unaffected. An explicit GOMAXPROCS in the environment wins.
func TuneRuntime() int {
	if os.Getenv("GOMAXPROCS") != "" {
		return runtime.GOMAXPROCS(0)
	}
	runtime.GOMAXPROCS(targetProcs(runtime.NumCPU()))
	return runtime.GOMAXPROCS(0)
}
