package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/linker"
)

type def struct {
	Label string `yaml:"label"`
	Min   string `yaml:"min,omitempty"`
}

// choosePrompter answers a Choose with a fixed index and records the options.
type choosePrompter struct {
	pick    int
	err     error
	title   string
	options []string
	asked   bool
}

func (c *choosePrompter) Confirm(string, bool) bool { return false }

func (c *choosePrompter) Choose(title string, options []string) (int, error) {
	c.asked = true
	c.title, c.options = title, options
	return c.pick, c.err
}

func TestConfirmReplace_identicalDefinitionsAskNothing(t *testing.T) {
	p := &choosePrompter{}
	action, err := confirmReplaceWith(p, "framework", "laravel", def{Label: "Laravel"}, def{Label: "Laravel"})
	if err != nil {
		t.Fatal(err)
	}
	if action != replaceSkip {
		t.Errorf("action = %v, want replaceSkip", action)
	}
	if p.asked {
		t.Error("identical definitions must not ask the user anything")
	}
}

// Without a terminal there is nobody to resolve the conflict, so the link keeps
// both sides as they are instead of failing. Driving a TUI form here used to
// abort the whole link with "could not open TTY", which made a project that
// commits its own framework_def impossible to link from the dashboard or an
// assistant.
func TestConfirmReplace_withoutAPrompterKeepsBothAndDoesNotFail(t *testing.T) {
	action, err := confirmReplaceWith(nil, "framework", "laravel", def{Label: "Laravel"}, def{Label: "Other"})
	if err != nil {
		t.Fatalf("a conflict with nobody to ask must not fail the link: %v", err)
	}
	if action != replaceSkip {
		t.Errorf("action = %v, want replaceSkip", action)
	}
}

func TestConfirmReplace_mapsTheChoiceOntoTheAction(t *testing.T) {
	cases := []struct {
		pick int
		want replaceAction
	}{
		{0, replaceFromProject},
		{1, replaceFromDisk},
		{2, replaceSkip},
		{9, replaceSkip}, // out of range is the conservative branch
		{-1, replaceSkip},
	}
	for _, c := range cases {
		p := &choosePrompter{pick: c.pick}
		action, err := confirmReplaceWith(p, "service", "mysql", def{Label: "a"}, def{Label: "b"})
		if err != nil {
			t.Fatal(err)
		}
		if action != c.want {
			t.Errorf("pick %d → %v, want %v", c.pick, action, c.want)
		}
	}
}

func TestConfirmReplace_offersAllThreeResolutions(t *testing.T) {
	p := &choosePrompter{}
	if _, err := confirmReplaceWith(p, "framework", "laravel", def{Label: "a"}, def{Label: "b"}); err != nil {
		t.Fatal(err)
	}
	if len(p.options) != 3 {
		t.Fatalf("options = %v, want three resolutions", p.options)
	}
	if p.title == "" {
		t.Error("the question needs a title naming the conflict")
	}
}

// The prompter is the seam; a real one is what the CLI supplies.
var _ linker.Prompter = (*choosePrompter)(nil)
