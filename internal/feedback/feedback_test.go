package feedback

import (
	"bytes"
	"errors"
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
