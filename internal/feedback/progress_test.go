package feedback

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderBar(t *testing.T) {
	cases := []struct {
		done, total int
		wantFilled  int
	}{
		{0, 10, 0},
		{5, 10, barWidth / 2},
		{10, 10, barWidth},
		{3, 0, barWidth},   // a zero total is a finished bar, never a divide by zero
		{20, 10, barWidth}, // more done than planned still stops at full
	}
	for _, c := range cases {
		got := renderBar(c.done, c.total)
		if filled := strings.Count(got, barFull); filled != c.wantFilled {
			t.Errorf("renderBar(%d, %d) has %d filled cells, want %d (%q)", c.done, c.total, filled, c.wantFilled, got)
		}
		if cells := len([]rune(got)); cells != barWidth {
			t.Errorf("renderBar(%d, %d) is %d cells wide, want %d", c.done, c.total, cells, barWidth)
		}
	}
}

// On a plain (non-animated) output there is no spinner to redraw, so each item
// gets its own line and the counts still add up.
func TestProgress_plainOutputLinePerItem(t *testing.T) {
	var buf bytes.Buffer
	restoreW := SetTestWriter(&buf)
	defer restoreW()
	restoreA := SetAnimated(false)
	defer restoreA()

	p := StartProgress("linking projects", 3)
	p.Step("alpha")
	p.Step("beta")
	p.Skip("gamma", "already registered")
	p.Done("3 done")

	out := buf.String()
	for _, want := range []string{"alpha", "beta", "gamma", "already registered", "3 done"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if p.Completed() != 2 {
		t.Errorf("Completed() = %d, want 2 (a skip is not a completion)", p.Completed())
	}
	if p.Skipped() != 1 {
		t.Errorf("Skipped() = %d, want 1", p.Skipped())
	}
}

func TestProgress_countsFailuresSeparately(t *testing.T) {
	var buf bytes.Buffer
	restoreW := SetTestWriter(&buf)
	defer restoreW()
	restoreA := SetAnimated(false)
	defer restoreA()

	p := StartProgress("linking projects", 2)
	p.Step("ok")
	p.Failed("broken", "no framework")
	p.Done("finished")

	if p.Completed() != 1 {
		t.Errorf("Completed() = %d, want 1", p.Completed())
	}
	if p.Failures() != 1 {
		t.Errorf("Failures() = %d, want 1", p.Failures())
	}
	if out := buf.String(); !strings.Contains(out, "no framework") {
		t.Errorf("the failure reason was not shown:\n%s", out)
	}
}

// A progress line must be usable from several goroutines, since resolving many
// projects at once is the reason it exists.
func TestProgress_concurrentStepsAreCounted(t *testing.T) {
	var buf bytes.Buffer
	restoreW := SetTestWriter(&buf)
	defer restoreW()
	restoreA := SetAnimated(false)
	defer restoreA()

	p := StartProgress("linking projects", 50)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() { p.Step("proj"); done <- struct{}{} }()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	p.Done("all")

	if p.Completed() != 50 {
		t.Errorf("Completed() = %d, want 50", p.Completed())
	}
}
