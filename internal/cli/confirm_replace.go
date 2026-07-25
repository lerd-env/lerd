package cli

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/geodro/lerd/internal/linker"
	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/yaml.v3"
)

var (
	addStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	delStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	metaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // blue
)

// replaceAction represents the user's choice when a definition conflict is found.
type replaceAction int

const (
	replaceSkip        replaceAction = iota // keep both as-is
	replaceFromProject                      // apply .lerd.yaml → disk
	replaceFromDisk                         // apply disk → .lerd.yaml
)

// replaceOptions are the resolutions offered, in the order they are presented.
// The index the user picks maps onto the action at the same position.
var replaceOptions = []struct {
	label  string
	action replaceAction
}{
	{"Use version from .lerd.yaml (update local definition)", replaceFromProject},
	{"Use local definition (update .lerd.yaml)", replaceFromDisk},
	{"Skip (keep both as-is)", replaceSkip},
}

// confirmReplace resolves a conflict between a definition on disk and the one a
// project committed, asking at the terminal when there is one.
func confirmReplace(kind, name string, existing, replacement interface{}) (replaceAction, error) {
	return confirmReplaceWith(linkPrompter(), kind, name, existing, replacement)
}

// confirmReplaceWith compares existing (on disk) and replacement (from
// .lerd.yaml) by marshalling both to YAML. Identical definitions resolve to
// replaceSkip without a word. Otherwise it prints a unified diff and asks which
// direction to sync.
//
// A nil prompter means nobody can answer — a link from the dashboard, an
// assistant, or a script. That keeps both sides as they are rather than failing
// the link, which is what driving a TUI form with no terminal used to do.
func confirmReplaceWith(prompt linker.Prompter, kind, name string, existing, replacement interface{}) (replaceAction, error) {
	oldYAML, err := yaml.Marshal(existing)
	if err != nil {
		return replaceSkip, err
	}
	newYAML, err := yaml.Marshal(replacement)
	if err != nil {
		return replaceSkip, err
	}

	if string(oldYAML) == string(newYAML) {
		return replaceSkip, nil // identical — nothing to do
	}

	if prompt == nil {
		// Unstyled: this branch is reached exactly when there is no terminal (the
		// dashboard, an assistant, a script), and lipgloss emits colour into a
		// pipe regardless, which would land as escape codes in their output.
		fmt.Printf("\n  ~ %s/%s differs from the one this project commits; keeping both as they are.\n", kind, name)
		fmt.Printf("    Run 'lerd link' in a terminal to choose which to keep.\n")
		return replaceSkip, nil
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(oldYAML)),
		B:        difflib.SplitLines(string(newYAML)),
		FromFile: fmt.Sprintf("%s/%s (current)", kind, name),
		ToFile:   fmt.Sprintf("%s/%s (.lerd.yaml)", kind, name),
		Context:  3,
	})
	if err != nil {
		return replaceSkip, err
	}

	fmt.Printf("\n%s %s/%s already exists and differs:\n\n", metaStyle.Render("~"), kind, name)
	for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			fmt.Println(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			fmt.Println(delStyle.Render(line))
		case strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			fmt.Println(metaStyle.Render(line))
		default:
			fmt.Println(line)
		}
	}
	fmt.Println()

	labels := make([]string, 0, len(replaceOptions))
	for _, o := range replaceOptions {
		labels = append(labels, o.label)
	}
	choice, err := prompt.Choose(fmt.Sprintf("How to resolve %s/%s?", kind, name), labels)
	if err != nil {
		return replaceSkip, err
	}
	if choice < 0 || choice >= len(replaceOptions) {
		return replaceSkip, nil
	}
	return replaceOptions[choice].action, nil
}
