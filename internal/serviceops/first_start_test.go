package serviceops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// firstStartFixture points lerd's data dir at a temp dir with a data
// directory for name holding the files a killed first start leaves behind.
func firstStartFixture(t *testing.T, name string) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := config.DataSubDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ibdata1"), []byte("half written"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevReady, prevStop, prevStart := firstStartReady, firstStartStop, firstStartStart
	t.Cleanup(func() { firstStartReady, firstStartStop, firstStartStart = prevReady, prevStop, prevStart })
	firstStartStop = func(string) error { return nil }
	firstStartStart = func(string) error { return nil }
	return dir
}

func asideDirs(t *testing.T, dir string) []string {
	t.Helper()
	m, _ := filepath.Glob(dir + ".incomplete-first-start-*")
	return m
}

// A first start killed while the engine initialised its data directory fails
// every start after it; that directory never held user data, so it is moved
// aside and the service comes up on a clean one.
func TestWaitReadyFirstStart_RecoversAnUnfinishedFirstStart(t *testing.T) {
	dir := firstStartFixture(t, "mysql")
	config.MarkFirstStartPending("mysql")
	calls := 0
	firstStartReady = func(string, time.Duration) error {
		calls++
		if calls == 1 {
			return errors.New("mysql did not become ready within 1m0s")
		}
		return nil
	}

	if err := waitReadyFirstStart("mysql", time.Second); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if got := asideDirs(t, dir); len(got) != 1 {
		t.Fatalf("aside dirs = %v, want the unfinished one moved aside", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "ibdata1")); err == nil {
		t.Error("the service restarted on the same half-written data")
	}
	if config.FirstStartPending("mysql") {
		t.Error("the pending mark stayed after the service became ready")
	}
}

// A data directory that has worked before, or that lerd did not create, is
// never moved: a failed start there is reported and nothing else.
func TestWaitReadyFirstStart_NeverTouchesDataThatWorkedBefore(t *testing.T) {
	dir := firstStartFixture(t, "mysql")
	firstStartReady = func(string, time.Duration) error { return errors.New("not ready") }

	err := waitReadyFirstStart("mysql", time.Second)

	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("err = %v, want the readiness error passed through", err)
	}
	if got := asideDirs(t, dir); len(got) != 0 {
		t.Errorf("moved data aside without a pending first start: %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "ibdata1")); err != nil {
		t.Error("the data directory was changed")
	}
}

func TestWaitReadyFirstStart_ReadyClearsThePendingMark(t *testing.T) {
	firstStartFixture(t, "mysql")
	config.MarkFirstStartPending("mysql")
	firstStartReady = func(string, time.Duration) error { return nil }

	if err := waitReadyFirstStart("mysql", time.Second); err != nil {
		t.Fatal(err)
	}
	if config.FirstStartPending("mysql") {
		t.Error("the pending mark stayed after the first successful start")
	}
}

// A service whose image never arrived wrote nothing, so an empty directory is
// not a half-written one: there is nothing to move aside or retry on.
func TestWaitReadyFirstStart_LeavesAnEmptyDirectoryAlone(t *testing.T) {
	dir := firstStartFixture(t, "valkey")
	if err := os.Remove(filepath.Join(dir, "ibdata1")); err != nil {
		t.Fatal(err)
	}
	config.MarkFirstStartPending("valkey")
	starts := 0
	firstStartStart = func(string) error { starts++; return nil }
	firstStartReady = func(string, time.Duration) error { return errors.New("valkey did not become ready") }

	if err := waitReadyFirstStart("valkey", time.Second); err == nil {
		t.Fatal("an unready service reported ready")
	}
	if got := asideDirs(t, dir); len(got) != 0 {
		t.Errorf("aside dirs = %v, want none for an empty directory", got)
	}
	if starts != 0 {
		t.Errorf("started %d more times, want no retry", starts)
	}
	if !config.FirstStartPending("valkey") {
		t.Error("first start no longer pending, a later start could not recover")
	}
}
