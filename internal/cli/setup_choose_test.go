package cli

import (
	"reflect"
	"testing"
)

// With no terminal the step picker cannot open, and failing on it left setup
// doing nothing; the default selection runs instead, as every other prompt does.
func TestChooseSetupSteps_withoutATerminalRunsTheDefaults(t *testing.T) {
	orig := promptableTTY
	t.Cleanup(func() { promptableTTY = orig })
	promptableTTY = func() bool { return false }

	steps := []setupStep{{label: "composer install", enabled: true}, {label: "db:seed"}, {label: "migrate", enabled: true}}
	got, err := chooseSetupSteps(steps, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"composer install", "migrate"}; !reflect.DeepEqual(got, want) {
		t.Errorf("selected %v, want the enabled steps %v", got, want)
	}
}

func TestChooseSetupSteps_allTakesEveryStep(t *testing.T) {
	steps := []setupStep{{label: "composer install", enabled: true}, {label: "db:seed"}}
	got, err := chooseSetupSteps(steps, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"composer install", "db:seed"}; !reflect.DeepEqual(got, want) {
		t.Errorf("selected %v, want every step %v", got, want)
	}
}
