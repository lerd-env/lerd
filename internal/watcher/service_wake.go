package watcher

import (
	"fmt"

	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/servicewake"
)

// The steps ensureForCaller takes, vars so tests can stand in for the engine
// and the containers.
var (
	wakeSocketWakeAsleep    = func(names []string) { idleEng.wakeServicesNow(names) }
	wakeSocketEnsureRunning = serviceops.EnsureServiceRunning
	wakeSocketReady         = serviceops.ServiceReady
)

// startServiceWakeSocket answers lerd processes that ask for a service, so the
// one manager here starts and probes each service once however many ask.
// Best-effort: without the socket each process brings services up itself.
func startServiceWakeSocket() {
	l, err := servicewake.Listen()
	if err != nil {
		fmt.Printf("[WARN] service wake socket: %v\n", err)
		return
	}
	go servicewake.Serve(l, wakeSocketReady, ensureForCaller)
}

// ensureForCaller brings a service up for a lerd process that asked. One
// idle-suspend put to sleep is woken by the engine, which also restores its
// sites and keeps its own bookkeeping, before every one is ensured ready.
func ensureForCaller(name string) error {
	if serviceAsleep(name) {
		wakeSocketWakeAsleep([]string{name})
	}
	return wakeSocketEnsureRunning(name)
}
