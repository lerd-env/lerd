package nginx

import (
	"errors"
	"testing"
	"time"
)

func TestReloadWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	n := 0
	err := reloadWithRetry(func() error {
		n++
		if n < 3 {
			return errors.New("cannot load certificate: No such file")
		}
		return nil
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("expected success once the reload settles, got %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}
}

func TestReloadWithRetry_ReturnsLastErrorOnTimeout(t *testing.T) {
	want := errors.New("persistent config error")
	err := reloadWithRetry(func() error { return want }, 300*time.Millisecond)
	if !errors.Is(err, want) {
		t.Fatalf("expected the last reload error, got %v", err)
	}
}

func TestReloadWithRetry_SucceedsFirstTry(t *testing.T) {
	n := 0
	err := reloadWithRetry(func() error { n++; return nil }, time.Second)
	if err != nil || n != 1 {
		t.Fatalf("expected one successful attempt, got n=%d err=%v", n, err)
	}
}
