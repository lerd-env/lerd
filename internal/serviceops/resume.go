package serviceops

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// resume is the one place a service is brought up and waited for. Callers here
// join the run in flight; other lerd processes queue on the service's lock and
// then find it ready or get the error that run left, instead of starting it again.
func resume(name string, bringUp func() error) error {
	return resumeAs(name, name, resumeReady, bringUp)
}

// resumeStart is resume for a start that does not wait until ready, as an
// explicit start from the dashboard or the CLI. It shares the service's lock but
// not its in-flight run, which promises a ready service a start cannot give.
func resumeStart(name string, start func() error) error {
	return resumeAs(name+" start", name, resumeActive, start)
}

// StartShared runs start for a service through the start every other caller
// of that service shares, for a caller that starts its unit its own way.
func StartShared(name string, start func() error) error { return resumeStart(name, start) }

// resumeAs runs bringUp once for the callers of key; settled says whether a run
// that went before already did what this caller needs.
func resumeAs(key, name string, settled func(string) bool, bringUp func() error) error {
	resumeCalls.Lock()
	if c, ok := resumeCalls.m[key]; ok {
		resumeCalls.Unlock()
		<-c.done
		return c.err
	}
	c := &resumeCall{done: make(chan struct{})}
	resumeCalls.m[key] = c
	resumeCalls.Unlock()

	c.err = resumeLocked(name, settled, bringUp)

	resumeCalls.Lock()
	delete(resumeCalls.m, key)
	resumeCalls.Unlock()
	close(c.done)
	return c.err
}

type resumeCall struct {
	done chan struct{}
	err  error
}

var resumeCalls = struct {
	sync.Mutex
	m map[string]*resumeCall
}{m: map[string]*resumeCall{}}

// resumeReady reports whether a service is up and answered its readiness
// probe in its current run, read fresh rather than from the status cache, so a
// caller that waited sees what the run before it did; a seam for tests.
var resumeReady = func(name string) bool {
	unit := "lerd-" + name
	podman.InvalidateUnitStatusCache(unit)
	if status, _ := podman.UnitStatus(unit); status != "active" {
		return false
	}
	id := activationIDFn(unit)
	return id != "" && readyActivation(name) == id
}

// ServiceReady reports whether a service is up and answered its probe in its
// current run, the check that lets an asker skip a start.
func ServiceReady(name string) bool { return resumeReady(name) }

// resumeActive reports whether a service's unit is up, read fresh, which is all
// an explicit start promises; a seam for tests.
var resumeActive = func(name string) bool {
	unit := "lerd-" + name
	podman.InvalidateUnitStatusCache(unit)
	status, _ := podman.UnitStatus(unit)
	return status == "active"
}

func resumeLocked(name string, settled func(string) bool, bringUp func() error) error {
	lock, err := openResumeLock(name)
	if err != nil {
		return bringUp() // no lock to share: behave as every caller did before
	}
	defer lock.Close()
	waitedFrom := time.Now()
	waited := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil
	if waited {
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
			return bringUp()
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	if waited {
		if settled(name) {
			return nil
		}
		if msg, ok := failedSince(name, waitedFrom); ok {
			return errors.New(msg)
		}
	}
	err = bringUp()
	recordResume(name, err)
	return err
}

func resumeLockDir() string { return filepath.Join(config.DataDir(), "service-locks") }

// resumeFile names a service's lock or failure file by a hash of its name: the
// name can come from a project's .env, so it never becomes part of a path.
func resumeFile(name, ext string) string {
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(resumeLockDir(), hex.EncodeToString(sum[:8])+ext)
}

func openResumeLock(name string) (*os.File, error) {
	if err := os.MkdirAll(resumeLockDir(), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(resumeFile(name, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
}

func failedFile(name string) string { return resumeFile(name, ".failed") }

// recordResume leaves a failed run's error for the callers that queued behind
// it, and clears it once a run succeeds.
func recordResume(name string, err error) {
	if err == nil {
		_ = os.Remove(failedFile(name))
		return
	}
	_ = os.WriteFile(failedFile(name), []byte(err.Error()), 0o644)
}

// failedSince returns the error a run left while the caller waited for the lock.
func failedSince(name string, since time.Time) (string, bool) {
	info, err := os.Stat(failedFile(name))
	if err != nil || info.ModTime().Before(since) {
		return "", false
	}
	b, err := os.ReadFile(failedFile(name))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}
