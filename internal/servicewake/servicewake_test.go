package servicewake

import (
	"errors"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// serve runs a watcher on a socket of its own for one test.
func serve(t *testing.T, ready func(string) bool, ensure func(string) error) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go Serve(l, ready, ensure)
}

func TestAsk_aReadyServiceAnswersAtOnce(t *testing.T) {
	var ensured atomic.Int32
	serve(t, func(string) bool { return true }, func(string) error { ensured.Add(1); return nil })
	starting := false
	handled, err := Ask("mysql", func() { starting = true })
	if !handled || err != nil || starting {
		t.Fatalf("handled=%v err=%v starting=%v, want handled, no error, no start", handled, err, starting)
	}
	if ensured.Load() != 1 {
		t.Errorf("ensured %d times, want once", ensured.Load())
	}
}

func TestAsk_aColdStartSaysSoBeforeItAnswers(t *testing.T) {
	serve(t, func(string) bool { return false }, func(string) error { time.Sleep(50 * time.Millisecond); return nil })
	var starting time.Time
	handled, err := Ask("mysql", func() { starting = time.Now() })
	if !handled || err != nil || starting.IsZero() {
		t.Fatalf("handled=%v err=%v starting=%v, want handled, no error, told it was starting", handled, err, starting)
	}
}

func TestAsk_aFailedStartReturnsTheWatchersReason(t *testing.T) {
	serve(t, func(string) bool { return false }, func(string) error { return errors.New("mysql would not start:\nport 3306 in use") })
	handled, err := Ask("mysql", nil)
	if !handled || err == nil || err.Error() != "mysql would not start:\nport 3306 in use" {
		t.Fatalf("handled=%v err=%v, want the watcher's reason", handled, err)
	}
}

func TestServe_refusesWhatIsNotAServiceName(t *testing.T) {
	var ensured atomic.Int32
	serve(t, func(string) bool { return false }, func(string) error { ensured.Add(1); return nil })
	for _, name := range []string{"", "../mysql", "my sql", "MySQL", "mysql\nensure redis"} {
		handled, err := Ask(name, nil)
		if !handled || err == nil {
			t.Errorf("%q: handled=%v err=%v, want refused", name, handled, err)
		}
	}
	if ensured.Load() != 0 {
		t.Errorf("ensured %d times for bad names", ensured.Load())
	}
}

// No watcher, or one that drops the request, is the caller's cue to bring the
// service up itself.
func TestAsk_withoutAnAnswerTheCallerDoesItItself(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if handled, err := Ask("mysql", nil); handled || err != nil {
		t.Fatalf("no watcher: handled=%v err=%v, want not handled", handled, err)
	}

	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err == nil {
			buf := make([]byte, 64)
			_, _ = c.Read(buf)
			c.Close()
		}
	}()
	if handled, err := Ask("mysql", nil); handled || err != nil {
		t.Fatalf("dropped request: handled=%v err=%v, want not handled", handled, err)
	}
}

func TestListen_onlyThisUserCanAsk(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	info, err := os.Stat(l.Addr().(*net.UnixAddr).Name)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("socket mode %o, want 600", perm)
	}
}
