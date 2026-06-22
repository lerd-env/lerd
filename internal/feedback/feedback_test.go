package feedback

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStepOKPlain(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	Start("detecting framework").OK("Laravel 11")

	got := buf.String()
	want := " → detecting framework… ✓ Laravel 11\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// StartOn renders to its own writer so a step can animate on the real stdout
// while the caller redirects the package target elsewhere (e.g. to capture a
// sub-step's output), without the step line leaking into that redirect.
func TestStartOnWritesToGivenWriter(t *testing.T) {
	var pkg, fixed bytes.Buffer
	defer SetTestWriter(&pkg)()

	StartOn(&fixed, "installing deps").OK("ok")

	if pkg.Len() != 0 {
		t.Errorf("StartOn wrote to the package target, want only the fixed writer: %q", pkg.String())
	}
	if got := fixed.String(); got != " → installing deps… ✓ ok\n" {
		t.Errorf("StartOn output = %q", got)
	}
}

func TestStepInfoAndFail(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	Start("writing vhost").Info("done")
	Start("provisioning TLS").Fail(errors.New("mkcert missing"))

	got := buf.String()
	if !strings.Contains(got, " → writing vhost… done\n") {
		t.Errorf("missing info line: %q", got)
	}
	if !strings.Contains(got, " → provisioning TLS… ✗ mkcert missing\n") {
		t.Errorf("missing fail line: %q", got)
	}
}

func TestLine(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	Line("php 8.4 · node 22 · nginx vhost written")

	if got := buf.String(); got != " → php 8.4 · node 22 · nginx vhost written\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSuccess(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	Success("linked", 1800*time.Millisecond)

	if got := buf.String(); got != " ✓ linked in 1.8s\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDone(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	Done("unparked sites")

	if got := buf.String(); got != " ✓ unparked sites\n" {
		t.Fatalf("got %q", got)
	}
}

func TestSummaryAligned(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	NewSummary().
		Row("Site", "https://acme.test").
		Row("PHP", "8.4.3 · FPM running").
		Row("DB", "mysql · cache redis").
		Print()

	want := "\n" +
		"  Site   https://acme.test\n" +
		"  PHP    8.4.3 · FPM running\n" +
		"  DB     mysql · cache redis\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestEmptySummaryPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	NewSummary().Print()

	if got := buf.String(); got != "" {
		t.Fatalf("expected no output, got %q", got)
	}
}

func TestValPlainPassthrough(t *testing.T) {
	defer SetTestWriter(&bytes.Buffer{})()
	if got := Val("8.4"); got != "8.4" {
		t.Fatalf("plain Val should not style, got %q", got)
	}
}

func TestHumanDur(t *testing.T) {
	cases := map[time.Duration]string{
		500 * time.Millisecond:  "500ms",
		1800 * time.Millisecond: "1.8s",
		2 * time.Second:         "2.0s",
	}
	for d, want := range cases {
		if got := humanDur(d); got != want {
			t.Errorf("humanDur(%v) = %q, want %q", d, got, want)
		}
	}
}

// withAnimatedBuffer forces animated mode against buf so the Live spinner path
// runs. SetTestWriter forces plain mode (which short-circuits Interrupt), so we
// set the package globals directly and restore them on cleanup.
func withAnimatedBuffer(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	mu.Lock()
	prevOut, prevColor := out, colorOn.Load()
	out = buf
	colorOn.Store(true)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		out = prevOut
		colorOn.Store(prevColor)
		mu.Unlock()
	})
}

func TestLiveInterrupt_SuppressesSpinnerWhilePaused(t *testing.T) {
	var buf bytes.Buffer
	withAnimatedBuffer(t, &buf)

	l := &Live{msg: "configuring .env"}
	buf.Reset()

	// A spinner tick that fires during the interrupt must not redraw, and the
	// callback's own output must reach the writer.
	l.Interrupt(func() {
		l.draw("⠙") // simulate the spinner goroutine racing the pause
		fmt.Fprint(&buf, "  Starting mysql...\n")
	})

	got := buf.String()
	if strings.Contains(got, "configuring .env") {
		t.Fatalf("spinner redrew while paused: %q", got)
	}
	if !strings.Contains(got, "Starting mysql...") {
		t.Fatalf("interrupt callback output missing: %q", got)
	}

	// After the interrupt the spinner resumes and draws normally again.
	buf.Reset()
	l.draw("⠹")
	if !strings.Contains(buf.String(), "configuring .env") {
		t.Fatalf("spinner did not resume after interrupt: %q", buf.String())
	}
}

func TestLiveInterrupt_PlainModeJustRunsFn(t *testing.T) {
	var buf bytes.Buffer
	defer SetTestWriter(&buf)()

	l := StartLive("configuring .env")
	ran := false
	l.Interrupt(func() { ran = true })
	if !ran {
		t.Fatal("Interrupt did not run fn in plain mode")
	}
}

// TestWarn_PausesActiveLine verifies that a Warn emitted while a live spinner is
// animating is routed through Interrupt: the spinner doesn't clobber it and the
// warning text reaches the writer. This is the env.go clobber regression.
func TestWarn_PausesActiveLine(t *testing.T) {
	var buf bytes.Buffer
	withAnimatedBuffer(t, &buf)

	l := &Live{msg: "configuring .env"}
	prev := liveActive.Swap(l)
	t.Cleanup(func() { liveActive.Store(prev) })

	buf.Reset()
	Warn("could not start %s: boom", "mysql")

	got := buf.String()
	if !strings.Contains(got, "could not start mysql") {
		t.Fatalf("warning text missing: %q", got)
	}
	// The line must have been cleared (Interrupt's \r\033[2K) so the spinner
	// redraw doesn't overwrite the warning.
	if !strings.Contains(got, "\r\033[2K") {
		t.Fatalf("warn did not pause/clear the live line: %q", got)
	}
}

// TestStartLive_TracksActiveAcrossNesting verifies StartLive registers the active
// line and Done/Fail restore the previous one, so a nested configure step doesn't
// leave the outer line unregistered (which would re-expose the clobber).
func TestStartLive_TracksActiveAcrossNesting(t *testing.T) {
	var buf bytes.Buffer
	withAnimatedBuffer(t, &buf)

	baseline := liveActive.Load()
	t.Cleanup(func() { liveActive.Store(baseline) })

	outer := StartLive("outer")
	if liveActive.Load() != outer {
		t.Fatal("StartLive did not register the outer line")
	}
	inner := StartLive("inner")
	if liveActive.Load() != inner {
		t.Fatal("nested StartLive did not register the inner line")
	}
	inner.Done()
	if liveActive.Load() != outer {
		t.Fatal("inner Done did not restore the outer line")
	}
	outer.Fail(nil)
	if liveActive.Load() != baseline {
		t.Fatal("outer Fail did not restore the baseline active line")
	}
}
