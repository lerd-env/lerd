package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// isolateSetupPlan keeps planning off the developer's real install: the global
// config, the site registry and the bun volume all resolve under XDG.
func isolateSetupPlan(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func planLabels(t *testing.T, dir string, skipOpen bool) []string {
	t.Helper()
	var labels []string
	for _, s := range planSetupSteps(dir, skipOpen) {
		labels = append(labels, s.label)
	}
	return labels
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func writePlanFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// The steps a directory would run are plannable without executing anything, so
// the dashboard can show the list the terminal selector shows.
func TestPlanSetupStepsOffersPackageManagerSteps(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{
		"composer.json": `{"require":{}}`,
		"package.json":  `{"scripts":{"build":"vite build"}}`,
	})

	labels := planLabels(t, dir, false)
	for _, want := range []string{"composer install", "npm install/ci", "npm run build", "lerd mcp:inject", "lerd open"} {
		if !hasLabel(labels, want) {
			t.Errorf("plan is missing %q: %v", want, labels)
		}
	}
}

// A project with no manifests has nothing to install, and skip-open drops the
// step that would open a browser on the host.
func TestPlanSetupStepsSkipsAbsentManifests(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()

	labels := planLabels(t, dir, true)
	for _, unwanted := range []string{"composer install", "npm install/ci", "lerd open"} {
		if hasLabel(labels, unwanted) {
			t.Errorf("plan should not offer %q for a bare directory: %v", unwanted, labels)
		}
	}
}

// Labels are what a caller selects by, so a plan may not repeat one.
func TestPlanSetupStepsHasUniqueLabels(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{
		"composer.json": `{"require":{}}`,
		"package.json":  `{"scripts":{"build":"vite build"}}`,
	})

	seen := map[string]bool{}
	for _, l := range planLabels(t, dir, false) {
		if seen[l] {
			t.Errorf("duplicate label %q", l)
		}
		seen[l] = true
	}
}

// --list-steps prints the plan as JSON with the defaults the selector would
// pre-tick, so a dashboard shows the same checkboxes.
func TestSetupStepPlanJSON(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"composer.json": `{"require":{}}`})

	out, err := json.Marshal(setupStepPlan(planSetupSteps(dir, true)))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Steps []struct {
			Label    string `json:"label"`
			Enabled  bool   `json:"enabled"`
			Optional bool   `json:"optional"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Steps) == 0 {
		t.Fatal("no steps in the JSON plan")
	}
	first := decoded.Steps[0]
	if first.Label != "composer install" {
		t.Errorf("first step = %q, want composer install", first.Label)
	}
	if !first.Enabled {
		t.Error("composer install should be pre-selected when vendor/ is missing")
	}
}

// The plan is re-derived for every `lerd setup --step`, and its steps are gated
// on state an earlier step or the watcher can flip. A queued label the plan no
// longer offers is work already done, so it is reported back and passed over
// rather than failing the caller and stopping the rest of its queue.
func TestSelectSetupStepsSkipsAStepThePlanNoLongerOffers(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"composer.json": `{"require":{}}`})

	steps := planSetupSteps(dir, true)
	selected, skipped := selectSetupSteps(steps, []string{"composer install"})
	if len(selected) != 1 || len(skipped) != 0 {
		t.Fatalf("an offered step was not selected: %d selected, skipped %v", len(selected), skipped)
	}

	selected, skipped = selectSetupSteps(steps, []string{"npm install/ci"})
	if len(selected) != 0 {
		t.Errorf("selected %d steps for a label the plan does not offer", len(selected))
	}
	if len(skipped) != 1 || skipped[0] != "npm install/ci" {
		t.Errorf("the vanished label was not reported back: %v", skipped)
	}
}

// Selection keeps the plan's order rather than the order the labels arrived in:
// composer install has to run before the build that needs it.
func TestSelectSetupStepsKeepsPlanOrder(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{
		"composer.json": `{"require":{}}`,
		"package.json":  `{"scripts":{"build":"vite build"}}`,
	})

	steps := planSetupSteps(dir, true)
	selected, skipped := selectSetupSteps(steps, []string{"npm run build", "composer install"})
	if len(skipped) != 0 {
		t.Fatalf("offered steps were skipped: %v", skipped)
	}
	if len(selected) != 2 {
		t.Fatalf("selected %d steps, want 2", len(selected))
	}
	if selected[0].label != "composer install" || selected[1].label != "npm run build" {
		t.Errorf("selection out of plan order: %q then %q", selected[0].label, selected[1].label)
	}
}

// HTTPS needs lerd's own DNS, so on a .localhost install the plan leaves the
// secure step out rather than offering one that can only fail and stop --all.
func TestPlanSetupStepsOmitsSecureWithoutManagedDNS(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()

	if !hasLabel(planLabels(t, dir, true), "lerd secure") {
		t.Fatal("managed DNS should offer lerd secure")
	}

	writePlanFixture(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "lerd"), map[string]string{
		"config.yaml": "dns:\n    enabled: false\n    tld: localhost\n",
	})
	if labels := planLabels(t, dir, true); hasLabel(labels, "lerd secure") {
		t.Errorf("a .localhost install should not offer lerd secure: %v", labels)
	}
}
