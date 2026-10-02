//go:build windows

package p9share

import (
	"fmt"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"
)

// Serve answers 9p for every share on its hvsock service, from any VM, until
// the process pid exits. Podman passes gvproxy's PID, so the server goes away
// with the machine's network the way its own does.
func Serve(shares []Share, pid int) error {
	exited, err := processExit(pid)
	if err != nil {
		return err
	}
	errc := make(chan error, len(shares))
	for _, s := range shares {
		svc, err := guid.FromString(s.Service)
		if err != nil {
			return fmt.Errorf("share %s: %w", s.Dir, err)
		}
		// A zero VMID is HV_GUID_WILDCARD: accept the connection from any VM.
		l, err := winio.ListenHvsock(&winio.HvsockAddr{ServiceID: svc})
		if err != nil {
			return fmt.Errorf("listening for %s on hvsock %s: %w", s.Dir, s.Service, err)
		}
		go func(dir string) {
			errc <- fmt.Errorf("serving %s: %w", dir, serve(l, dir))
		}(s.Dir)
	}
	select {
	case <-exited:
		return nil
	case err := <-errc:
		return err
	}
}

// processExit returns a channel closed once pid has exited.
func processExit(pid int) (<-chan struct{}, error) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil, fmt.Errorf("watching PID %d: %w", pid, err)
	}
	done := make(chan struct{})
	go func() {
		defer windows.CloseHandle(h)                     //nolint:errcheck
		windows.WaitForSingleObject(h, windows.INFINITE) //nolint:errcheck
		close(done)
	}()
	return done, nil
}
