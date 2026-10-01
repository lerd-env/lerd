package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitContains(t *testing.T, b *syncBuf, want string) {
	t.Helper()
	for i := 0; i < 60; i++ {
		if strings.Contains(b.String(), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("output never contained %q, got %q", want, b.String())
}

func TestTailFileReplaysTailThenFollows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unit.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := &syncBuf{}
	done := make(chan struct{})
	go func() { defer close(done); _ = tailFile(ctx, path, 2, out) }()

	waitContains(t, out, "four\n")
	if got := out.String(); strings.Contains(got, "one") || strings.Contains(got, "two") {
		t.Errorf("only the last 2 lines belong in the replay, got %q", got)
	}

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("five\nsi")
	waitContains(t, out, "five\n")
	_, _ = f.WriteString("x\n")
	f.Close()
	waitContains(t, out, "six\n")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tailFile did not stop on context cancel")
	}
}

func TestTailFileWaitsForTheFileToAppear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "late.log")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &syncBuf{}
	go func() { _ = tailFile(ctx, path, 5, out) }()
	time.Sleep(400 * time.Millisecond)
	if err := os.WriteFile(path, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	waitContains(t, out, "hello\n")
}
