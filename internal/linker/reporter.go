package linker

import (
	"fmt"
	"io"
)

// Reporter renders a link's progress. The CLI backs it with its feedback layer;
// a daemon or an assistant backs it with plain lines or nothing at all.
type Reporter interface {
	// Step announces work that is starting and returns the handle that closes it.
	Step(label string) Step
	// Line reports something worth showing that is not a step.
	Line(msg string)
	// Warn reports a problem that did not stop the link.
	Warn(format string, a ...any)
	// Val styles a value inside a message, so the linker can compose detail
	// strings without knowing how the caller renders them.
	Val(s string) string
}

// Step is one unit of announced work, closed by exactly one of its methods.
type Step interface {
	OK(detail string)
	Fail(err error)
}

// NopReporter discards everything, for callers that report on their own.
type NopReporter struct{}

func (NopReporter) Step(string) Step    { return nopStep{} }
func (NopReporter) Line(string)         {}
func (NopReporter) Warn(string, ...any) {}
func (NopReporter) Val(s string) string { return s }

type nopStep struct{}

func (nopStep) OK(string)  {}
func (nopStep) Fail(error) {}

// TextReporter writes plain lines, for the watcher and other log-only callers.
type TextReporter struct{ W io.Writer }

func (t TextReporter) Step(label string) Step { return textStep{w: t.W, label: label} }

func (t TextReporter) Line(msg string) { fmt.Fprintln(t.W, msg) }

func (t TextReporter) Warn(format string, a ...any) {
	fmt.Fprintf(t.W, "[WARN] "+format+"\n", a...)
}

func (TextReporter) Val(s string) string { return s }

type textStep struct {
	w     io.Writer
	label string
}

func (s textStep) OK(detail string) {
	if detail == "" {
		fmt.Fprintln(s.w, s.label)
		return
	}
	fmt.Fprintf(s.w, "%s: %s\n", s.label, detail)
}

func (s textStep) Fail(err error) { fmt.Fprintf(s.w, "[WARN] %s: %v\n", s.label, err) }
