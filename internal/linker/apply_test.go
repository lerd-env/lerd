package linker

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// fakePrompter answers every question the same way and records what it was asked.
type fakePrompter struct {
	answer bool
	asked  []string
}

func (f *fakePrompter) Confirm(question string, _ bool) bool {
	f.asked = append(f.asked, question)
	return f.answer
}

func (f *fakePrompter) Choose(title string, _ []string) (int, error) {
	f.asked = append(f.asked, title)
	return 0, nil
}

// recorder collects everything a link reported.
type recorder struct {
	lines []string
	warns []string
	steps []string
	fails []string
}

func (r *recorder) Step(label string) Step {
	r.steps = append(r.steps, label)
	return &recStep{r: r, label: label}
}
func (r *recorder) Line(msg string) { r.lines = append(r.lines, msg) }
func (r *recorder) Warn(format string, a ...any) {
	r.warns = append(r.warns, fmt.Sprintf(format, a...))
}
func (r *recorder) Val(s string) string { return s }

func (r *recorder) saw(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

type recStep struct {
	r     *recorder
	label string
}

func (s *recStep) OK(string)      {}
func (s *recStep) Fail(err error) { s.r.fails = append(s.r.fails, s.label) }

func clampedPlan() *Plan {
	return &Plan{
		Mode:           ModeFPM,
		PHPMin:         "8.2",
		PHPMax:         "8.4",
		PHPSuggestion:  "8.4",
		FrameworkLabel: "Laravel 11",
		Site:           config.Site{Name: "myapp", PHPVersion: "8.2"},
	}
}

func TestOfferBetterPHP_adoptsTheVersionWhenTheOfferIsAccepted(t *testing.T) {
	plan := clampedPlan()
	prompt := &fakePrompter{answer: true}
	built := ""
	deps := Deps{EnsureFPMQuadlet: func(v string) error { built = v; return nil }}
	r := &recorder{}

	if err := offerBetterPHP(plan, Policy{Prompt: prompt, ImageBuild: true}, deps, r); err != nil {
		t.Fatal(err)
	}
	if built != "8.4" {
		t.Errorf("built %q, want 8.4", built)
	}
	if plan.Site.PHPVersion != "8.4" {
		t.Errorf("php = %q, want 8.4 after accepting the offer", plan.Site.PHPVersion)
	}
}

func TestOfferBetterPHP_keepsTheCurrentVersionWhenDeclined(t *testing.T) {
	plan := clampedPlan()
	deps := Deps{EnsureFPMQuadlet: func(string) error { t.Fatal("must not build after a decline"); return nil }}

	if err := offerBetterPHP(plan, Policy{Prompt: &fakePrompter{}, ImageBuild: true}, deps, &recorder{}); err != nil {
		t.Fatal(err)
	}
	if plan.Site.PHPVersion != "8.2" {
		t.Errorf("php = %q, want the version it already had", plan.Site.PHPVersion)
	}
}

func TestOfferBetterPHP_keepsTheCurrentVersionWhenTheBuildFails(t *testing.T) {
	plan := clampedPlan()
	deps := Deps{EnsureFPMQuadlet: func(string) error { return errors.New("no network") }}
	r := &recorder{}

	if err := offerBetterPHP(plan, Policy{Prompt: &fakePrompter{answer: true}, ImageBuild: true}, deps, r); err != nil {
		t.Fatalf("a failed build must not fail the link: %v", err)
	}
	if plan.Site.PHPVersion != "8.2" {
		t.Errorf("php = %q, want the version it already had", plan.Site.PHPVersion)
	}
	if len(r.fails) == 0 {
		t.Error("the failed build was not reported")
	}
}

// Without a prompter there is nobody to ask, so the clamp is reported rather
// than a PHP image being built behind the user's back.
func TestOfferBetterPHP_withoutAPrompterReportsTheClampAndBuildsNothing(t *testing.T) {
	plan := clampedPlan()
	deps := Deps{EnsureFPMQuadlet: func(string) error { t.Fatal("must not build without consent"); return nil }}
	r := &recorder{}

	if err := offerBetterPHP(plan, Policy{ImageBuild: true}, deps, r); err != nil {
		t.Fatal(err)
	}
	if plan.Site.PHPVersion != "8.2" {
		t.Errorf("php = %q, want 8.2", plan.Site.PHPVersion)
	}
	if !r.saw(r.lines, "supports 8.2–8.4") {
		t.Errorf("clamp was not reported, lines = %v", r.lines)
	}
}

func TestOfferBetterPHP_saysNothingWhenTheVersionIsUnconstrained(t *testing.T) {
	plan := &Plan{Mode: ModeFPM, Site: config.Site{PHPVersion: "8.4"}}
	r := &recorder{}

	if err := offerBetterPHP(plan, Policy{}, Deps{}, r); err != nil {
		t.Fatal(err)
	}
	if len(r.lines) != 0 {
		t.Errorf("lines = %v, want none", r.lines)
	}
}

func proxyPlan(command string) *Plan {
	return &Plan{
		Mode:         ModeHostProxy,
		ProxyCommand: command,
		Site:         config.Site{Name: "web", HostPort: 5173, HostCommand: command},
	}
}

// A caller that does not run repository commands still gets the proxy vhost,
// but the dev command is dropped rather than supervised.
func TestGateProxyCommand_dropsTheCommandWhenRepoCommandsAreWithheld(t *testing.T) {
	plan := proxyPlan("npm run dev")
	r := &recorder{}

	if err := gateProxyCommand(plan, WatcherPolicy(), r); err != nil {
		t.Fatalf("withholding the command must not fail the link: %v", err)
	}
	if plan.Site.HostCommand != "" {
		t.Errorf("host command = %q, want it cleared", plan.Site.HostCommand)
	}
	if !r.saw(r.warns, "does not run repository commands") {
		t.Errorf("the drop was not reported, warns = %v", r.warns)
	}
}

func TestGateProxyCommand_proxyOnlyNeedsNoConsent(t *testing.T) {
	plan := proxyPlan("")
	if err := gateProxyCommand(plan, WatcherPolicy(), &recorder{}); err != nil {
		t.Fatalf("a proxy-only site supervises nothing: %v", err)
	}
	if plan.Site.HostCommand != "" {
		t.Errorf("host command = %q, want empty", plan.Site.HostCommand)
	}
}

func TestProvisionLabels(t *testing.T) {
	r := &recorder{}
	site := config.Site{PHPVersion: "8.4", NodeVersion: "22", ContainerPort: 3000}

	cases := map[Mode]string{
		ModeCustomContainer: "",
		ModeHostProxy:       "",
		ModeCustomFPM:       "building custom FPM image",
		ModeFrankenPHP:      "provisioning FrankenPHP runtime",
		ModeFPM:             "provisioning PHP-FPM runtime",
	}
	for mode, want := range cases {
		label, detail := provisionLabels(&Plan{Mode: mode}, site, r)
		if label != want {
			t.Errorf("%s label = %q, want %q", mode, label, want)
		}
		if want == "" && detail != "" {
			t.Errorf("%s detail = %q, want empty alongside an empty label", mode, detail)
		}
	}
}

func TestPolicies_capabilities(t *testing.T) {
	cli := CLIPolicy("", false, &fakePrompter{})
	if !cli.ProjectWrites || !cli.Services || !cli.Certs || !cli.RepoCommands || !cli.ImageBuild {
		t.Errorf("the CLI policy must grant everything: %+v", cli)
	}
	if cli.SkipRegistered {
		t.Error("a user-invoked link re-links an already registered path")
	}

	w := WatcherPolicy()
	if w.ProjectWrites || w.Services || w.Certs || w.RepoCommands || w.ImageBuild || w.Prompt != nil {
		t.Errorf("the watcher policy must grant nothing: %+v", w)
	}
	if !w.SkipRegistered {
		t.Error("the watcher must skip an already registered path")
	}
}
