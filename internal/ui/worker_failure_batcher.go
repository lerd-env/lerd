package ui

import (
	"sync"
	"time"

	"github.com/geodro/lerd/internal/workerheal"
)

// workerFailureBatchDelay is the settle window between the first new failure
// in a burst and the grouped dispatch. It collapses systemd cascades
// (start-limit storms, OOM kills hitting several queues at once) into one
// notification, and it gives the units time to come back on their own: worker
// units carry RestartSec=5, so a worker that crashed once is usually active
// again within a cycle or two. Notifying at five seconds meant most alerts
// described a worker that had already recovered by the time anyone opened the
// dashboard, so the window is long enough for several restart attempts to play
// out and only what is still broken at the end of it gets announced.
var workerFailureBatchDelay = 30 * time.Second

// workerFailureDispatch is the sink used by the batcher. Production wires it
// to dispatchNotification; tests override it to observe the grouped payload
// without exercising the push subsystem.
var workerFailureDispatch = func(ws []workerheal.UnhealthyWorker) {
	dispatchNotification(notificationForWorkerFailures(ws))
}

// workerFailureDetect re-reads current worker health at flush time. Swappable
// for tests.
var workerFailureDetect = workerheal.Detect

// stillUnhealthy drops queued failures that recovered during the settle window
// and refreshes the state of those that did not, so the notification describes
// the workers as they are when it is sent rather than when they first tripped.
// A detector error keeps the whole batch: failing to confirm is not evidence of
// recovery, and staying silent would be the worse mistake.
func stillUnhealthy(queued []workerheal.UnhealthyWorker) []workerheal.UnhealthyWorker {
	current, err := workerFailureDetect()
	if err != nil {
		return queued
	}
	byUnit := make(map[string]workerheal.UnhealthyWorker, len(current))
	for _, w := range current {
		byUnit[w.Unit] = w
	}
	out := make([]workerheal.UnhealthyWorker, 0, len(queued))
	for _, q := range queued {
		if w, ok := byUnit[q.Unit]; ok {
			out = append(out, w)
		}
	}
	return out
}

var (
	workerFailureBatchMu    sync.Mutex
	pendingWorkerFailures   = map[string]workerheal.UnhealthyWorker{}
	workerFailureFlushTimer *time.Timer
)

// queueWorkerFailureNotifications buffers the watcher's new-failure delta
// and arms a single flush timer. The timer is intentionally not reset when
// more failures arrive inside the window, so the grouped push lands at most
// workerFailureBatchDelay after the *first* failure in the batch even if
// new failures keep trickling in.
func queueWorkerFailureNotifications(ws []workerheal.UnhealthyWorker) {
	if len(ws) == 0 {
		return
	}
	workerFailureBatchMu.Lock()
	for _, w := range ws {
		pendingWorkerFailures[w.Unit] = w
	}
	if workerFailureFlushTimer == nil {
		workerFailureFlushTimer = time.AfterFunc(workerFailureBatchDelay, flushPendingWorkerFailures)
	}
	workerFailureBatchMu.Unlock()
}

func flushPendingWorkerFailures() {
	workerFailureBatchMu.Lock()
	ws := make([]workerheal.UnhealthyWorker, 0, len(pendingWorkerFailures))
	for _, w := range pendingWorkerFailures {
		ws = append(ws, w)
	}
	pendingWorkerFailures = map[string]workerheal.UnhealthyWorker{}
	workerFailureFlushTimer = nil
	workerFailureBatchMu.Unlock()
	if len(ws) == 0 {
		return
	}
	if ws = stillUnhealthy(ws); len(ws) == 0 {
		return
	}
	workerFailureDispatch(ws)
}
