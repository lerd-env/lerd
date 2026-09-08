package cli

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/store"
)

// defaultScaffoldFramework is the framework `lerd new` scaffolds when nothing
// asks for another: the answer the wizard starts on, and the one a run with no
// terminal keeps using so scripts and CI behave as they always have.
const defaultScaffoldFramework = "laravel"

// scaffoldChoice is one framework the wizard can offer: the store name a
// definition resolves by, the label to show, and the majors available with the
// newest first.
type scaffoldChoice struct {
	Name     string
	Label    string
	Versions []string
	Latest   string

	// sawDefinition records that a definition for this framework was read here,
	// so creatable means something. creatable records that one of them carries a
	// create command. A framework published but not installed leaves both false:
	// nothing about it is known yet, and it is offered rather than hidden.
	sawDefinition bool
	creatable     bool
	// scaffolds answers, for each major a definition was read for, whether that
	// major can start a project. A major missing from it was never inspected.
	scaffolds map[string]bool
}

// canScaffold reports whether the wizard should offer this framework. Not every
// definition can start a project: the store publishes frameworks whose install
// is not a composer create-project at all, and offering one only to refuse it
// after the questions is worse than never listing it.
func (c scaffoldChoice) canScaffold() bool {
	if c.creatable || !c.sawDefinition {
		return true
	}
	// A major whose definition is not installed says nothing either way, so the
	// framework is only dropped once every major it publishes has been read and
	// none of them can scaffold.
	for _, v := range c.Versions {
		if _, inspected := c.scaffolds[v]; !inspected {
			return true
		}
	}
	return false
}

// scaffoldCatalogue lists the frameworks `lerd new` can scaffold. It reads the
// definitions `lerd framework list` shows, which install seeds from the store,
// and folds in anything the cached index publishes that this machine has not
// installed yet, so a framework whose definition arrives on selection is still
// offered. Order follows the index, whose sequence is the store's to choose,
// with installed-only names after it.
func scaffoldCatalogue() []scaffoldChoice {
	byName := map[string]*scaffoldChoice{}

	add := func(name, label string) *scaffoldChoice {
		if c, ok := byName[name]; ok {
			if c.Label == "" {
				c.Label = label
			}
			return c
		}
		c := &scaffoldChoice{Name: name, Label: label}
		byName[name] = c
		return c
	}

	var published []string
	if idx, err := store.CachedIndex(); err == nil {
		for _, e := range idx.Frameworks {
			c := add(e.Name, e.Label)
			c.Latest = e.Latest
			for _, v := range e.Versions {
				c.Versions = appendVersion(c.Versions, v)
			}
			published = append(published, e.Name)
		}
	}

	var localOnly []string
	for _, info := range config.ListFrameworksDetailed() {
		if _, known := byName[info.Name]; !known {
			localOnly = append(localOnly, info.Name)
		}
		c := add(info.Name, info.Label)
		c.Versions = appendVersion(c.Versions, info.Version)
		c.sawDefinition = true
		c.scaffolds = config.FrameworkScaffoldSupport(info.Name)
		if info.Create != "" {
			c.creatable = true
		}
		for _, scaffolds := range c.scaffolds {
			if scaffolds {
				c.creatable = true
			}
		}
	}
	sort.Strings(localOnly)

	var out []scaffoldChoice
	for _, name := range append(published, localOnly...) {
		c := byName[name]
		if !c.canScaffold() {
			continue
		}
		sortFrameworkVersionsDesc(c.Versions)
		c.Versions = versionsThatScaffold(c.Versions, c.scaffolds)
		if c.Latest == "" && len(c.Versions) > 0 {
			c.Latest = c.Versions[0]
		}
		if c.Label == "" {
			c.Label = c.Name
		}
		out = append(out, *c)
	}
	return out
}

// FrameworkChoice is one framework a new project can be scaffolded from, as the
// dashboard's create step sees it.
type FrameworkChoice struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Versions []string `json:"versions,omitempty"`
	Latest   string   `json:"latest,omitempty"`
}

// FrameworkCatalogue lists what a new project can be scaffolded from: the same
// catalogue the terminal wizard offers, so both ask from the store rather than
// from a list of their own.
func FrameworkCatalogue() []FrameworkChoice {
	catalogue := scaffoldCatalogue()
	out := make([]FrameworkChoice, 0, len(catalogue))
	for _, c := range catalogue {
		out = append(out, FrameworkChoice{Name: c.Name, Label: c.Label, Versions: c.Versions, Latest: c.Latest})
	}
	return out
}

// appendVersion adds a major to a version list, skipping blanks (a legacy
// unversioned definition) and repeats.
func appendVersion(versions []string, v string) []string {
	if v == "" {
		return versions
	}
	for _, have := range versions {
		if have == v {
			return versions
		}
	}
	return append(versions, v)
}

// versionsThatScaffold drops the majors whose definition was read here and
// carries no create command, so the wizard never asks a question it would only
// refuse to act on. A major with no definition here stays: nothing is known
// about it yet, and it is fetched when picked.
func versionsThatScaffold(versions []string, scaffolds map[string]bool) []string {
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		if canScaffold, inspected := scaffolds[v]; inspected && !canScaffold {
			continue
		}
		out = append(out, v)
	}
	return out
}

// sortFrameworkVersionsDesc orders majors newest first, numerically, so 9 sits
// below 12 rather than above it the way a string comparison would put it.
func sortFrameworkVersionsDesc(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		a, aErr := strconv.Atoi(versions[i])
		b, bErr := strconv.Atoi(versions[j])
		if aErr != nil || bErr != nil {
			return versions[i] > versions[j]
		}
		return a > b
	})
}

// scaffoldChoiceByName returns the catalogue entry for a name, or nil.
func scaffoldChoiceByName(catalogue []scaffoldChoice, name string) *scaffoldChoice {
	for i := range catalogue {
		if catalogue[i].Name == name {
			return &catalogue[i]
		}
	}
	return nil
}

// newShouldAskFramework reports whether `lerd new` should ask which framework to
// scaffold. A run that named one has already answered, and a run with no
// terminal must never block on the question.
func newShouldAskFramework(interactive, frameworkGiven bool) bool {
	return interactive && !frameworkGiven
}

// frameworkSelectOptions renders the catalogue as picker options, labelled the
// way lerd framework list names each framework.
func frameworkSelectOptions(catalogue []scaffoldChoice) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(catalogue))
	for _, c := range catalogue {
		opts = append(opts, huh.NewOption(c.Label, c.Name))
	}
	return opts
}

// versionSelectOptions renders a framework's majors as picker options, marking
// the one the store publishes as current.
func versionSelectOptions(c scaffoldChoice) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(c.Versions))
	for _, v := range c.Versions {
		label := v
		if v == c.Latest {
			label = v + " (latest)"
		}
		opts = append(opts, huh.NewOption(label, v))
	}
	return opts
}

// initialScaffoldFramework picks which entry the framework question starts on:
// the command's long-standing default when the catalogue offers it, otherwise
// the first thing it does offer.
func initialScaffoldFramework(catalogue []scaffoldChoice) string {
	if scaffoldChoiceByName(catalogue, defaultScaffoldFramework) != nil {
		return defaultScaffoldFramework
	}
	if len(catalogue) > 0 {
		return catalogue[0].Name
	}
	return defaultScaffoldFramework
}

// validateProjectName rejects an answer that cannot become a directory, so the
// wizard says so at the prompt rather than failing several steps later.
func validateProjectName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("enter a name for the project")
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("the name cannot contain a NUL byte")
	}
	return nil
}

// askProjectName prompts for the target when the command was called without one.
func askProjectName() (string, error) {
	name := ""
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Project name").
			Description("A path works too, relative to here or absolute").
			Value(&name).
			Validate(validateProjectName),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run()
	return strings.TrimSpace(name), err
}

// askScaffoldFramework prompts for the framework and, when it has more than one
// major published, for which one. Returns the store name and version to resolve
// the definition by; an empty version means the latest.
func askScaffoldFramework(catalogue []scaffoldChoice) (string, string, error) {
	if len(catalogue) == 0 {
		return defaultScaffoldFramework, "", nil
	}

	name := initialScaffoldFramework(catalogue)
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Framework").
			Options(frameworkSelectOptions(catalogue)...).
			Value(&name),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return "", "", err
	}

	choice := scaffoldChoiceByName(catalogue, name)
	if choice == nil || len(choice.Versions) < 2 {
		return name, "", nil
	}

	version := choice.Latest
	if !slices.Contains(choice.Versions, version) {
		version = choice.Versions[0]
	}
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(choice.Label + " version").
			Options(versionSelectOptions(*choice)...).
			Value(&version),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return "", "", err
	}
	// The latest needs no pin: resolving without one follows the store as it
	// publishes new majors instead of freezing on the one listed today.
	if version == choice.Latest {
		version = ""
	}
	return name, version, nil
}
