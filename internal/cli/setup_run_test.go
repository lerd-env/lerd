package cli

import (
	"errors"
	"testing"
)

// Unattended setup has nobody to answer "continue?", so a failed step must end
// the run with an error instead of waiting on stdin forever.
func TestRunSelectedStepsUnattendedAbortsWithoutAsking(t *testing.T) {
	ranAfter := false
	steps := []setupStep{
		{label: "db:seed", run: func() error { return errors.New("boom") }},
		{label: "queue:start", run: func() error { ranAfter = true; return nil }},
	}
	sel := selectAll(steps)

	err := runSelectedSteps(steps, sel, func() bool { t.Fatal("unattended run must not prompt"); return false }, true)

	if err == nil {
		t.Fatal("want an error naming the failed step")
	}
	if ranAfter {
		t.Error("steps after the failure should not run")
	}
}

func TestRunSelectedStepsInteractiveContinuesOnYes(t *testing.T) {
	ranAfter := false
	steps := []setupStep{
		{label: "db:seed", run: func() error { return errors.New("boom") }},
		{label: "queue:start", run: func() error { ranAfter = true; return nil }},
	}

	if err := runSelectedSteps(steps, selectAll(steps), func() bool { return true }, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ranAfter {
		t.Error("answering yes should run the remaining steps")
	}
}
