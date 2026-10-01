package cli

import (
	"slices"
	"testing"
)

func serviceSteps() []setupStep {
	return []setupStep{
		{label: "composer install"},
		{label: "queue:start", service: "redis"},
		{label: "relt:start", service: "beanstalkd"},
		{label: "horizon:start", service: "redis"},
	}
}

func selectAll(steps []setupStep) map[string]bool {
	sel := map[string]bool{}
	for _, s := range steps {
		sel[s.label] = true
	}
	return sel
}

func runningOnly(names ...string) func(string) bool {
	return func(n string) bool { return slices.Contains(names, n) }
}

// --all is the unattended path: a missing service is installed and started
// without asking, once, however many workers need it.
func TestResolveSetupServicesAutoInstallsMissing(t *testing.T) {
	steps := serviceSteps()
	sel := selectAll(steps)
	var ensured []string
	confirm := func(string) bool { t.Fatal("--all must not prompt"); return false }

	resolveSetupServices(steps, sel, true, runningOnly("redis"), confirm, func(n string) error {
		ensured = append(ensured, n)
		return nil
	})

	if !slices.Equal(ensured, []string{"beanstalkd"}) {
		t.Fatalf("ensured %v, want [beanstalkd]", ensured)
	}
	if !sel["relt:start"] {
		t.Error("worker whose service was installed should stay selected")
	}
}

func TestResolveSetupServicesAsksAndInstallsOnYes(t *testing.T) {
	steps := serviceSteps()
	sel := selectAll(steps)
	var asked, ensured []string

	resolveSetupServices(steps, sel, false, runningOnly(), func(q string) bool {
		asked = append(asked, q)
		return true
	}, func(n string) error {
		ensured = append(ensured, n)
		return nil
	})

	if len(asked) != 2 || !slices.Equal(ensured, []string{"redis", "beanstalkd"}) {
		t.Fatalf("asked %d times, ensured %v, want each missing service once", len(asked), ensured)
	}
}

// Declining leaves the service alone and drops the workers that need it, so
// setup doesn't stop on a worker the user already chose not to run.
func TestResolveSetupServicesDeclineSkipsItsWorkers(t *testing.T) {
	steps := serviceSteps()
	sel := selectAll(steps)

	resolveSetupServices(steps, sel, false, runningOnly("beanstalkd"),
		func(string) bool { return false },
		func(string) error { t.Fatal("declined service must not be installed"); return nil })

	if sel["queue:start"] || sel["horizon:start"] {
		t.Errorf("workers needing the declined service should be deselected: %v", sel)
	}
	if !sel["relt:start"] || !sel["composer install"] {
		t.Errorf("unrelated steps should stay selected: %v", sel)
	}
}

func TestResolveSetupServicesIgnoresUnselectedSteps(t *testing.T) {
	steps := serviceSteps()
	sel := map[string]bool{"composer install": true}

	resolveSetupServices(steps, sel, true, runningOnly(), nil,
		func(n string) error { t.Fatalf("%s installed for an unselected worker", n); return nil })
}
