package config

import (
	"fmt"
	"slices"
	"strings"
)

// TestRunner is how a framework or a composer package runs a project's tests.
// The command runs the whole suite; a file is appended to it.
type TestRunner struct {
	// Command is argv split on spaces, no shell, and starts with php so it runs
	// wherever the site's PHP does.
	Command string `yaml:"command"`
	// Filter is the argument that narrows a run to tests matching {filter}.
	// Empty means the runner has none.
	Filter string `yaml:"filter,omitempty"`
	// Over names the packages this one is built on, whose runners it replaces.
	Over []string `yaml:"over,omitempty"`
	// Check gates the runner on the project, as a worker's does.
	Check *FrameworkRule `yaml:"check,omitempty"`
}

// Argv is the command line for a run, whole suite when file and filter are empty.
func (r *TestRunner) Argv(file, filter string) ([]string, error) {
	argv := strings.Fields(r.Command)
	if filter != "" {
		if r.Filter == "" {
			return nil, fmt.Errorf("%q cannot run tests by name, pass a file instead", r.Command)
		}
		for _, part := range strings.Fields(r.Filter) {
			argv = append(argv, strings.ReplaceAll(part, "{filter}", filter))
		}
	}
	if file != "" {
		argv = append(argv, file)
	}
	return argv, nil
}

// TestRunnerForDir returns how the project in dir runs its tests: its
// framework's own command, then the store package the project requires. nil
// when nothing declares one.
func TestRunnerForDir(dir string) (*TestRunner, error) {
	fw := frameworkOfDir(dir)
	r := packageTestRunner(fw, dir)
	if fw != nil && fw.Test != nil && fw.Test.applies(dir) {
		r = fw.Test
	}
	if r == nil {
		return nil, nil
	}
	if f := strings.Fields(r.Command); len(f) < 2 || f[0] != "php" {
		return nil, fmt.Errorf("test command %q is not one this lerd can run, it must start with php", r.Command)
	}
	return r, nil
}

func (r *TestRunner) applies(dir string) bool {
	return r.Check == nil || MatchesRule(dir, *r.Check)
}

func packageTestRunner(fw *Framework, dir string) *TestRunner {
	type candidate struct {
		name   string
		runner *TestRunner
	}
	var found []candidate
	var replaced []string
	for _, entry := range cachedStorePackages() {
		if !ComposerHasInstalled(dir, entry.Name) {
			continue
		}
		pkg := resolveStorePackage(entry, dir)
		if pkg == nil || pkg.Test == nil || !pkg.Test.applies(dir) || (len(pkg.Frameworks) > 0 && !pkg.AppliesTo(fw)) {
			continue
		}
		found = append(found, candidate{entry.Name, pkg.Test})
		replaced = append(replaced, pkg.Test.Over...)
	}
	for _, c := range found {
		if !slices.Contains(replaced, c.name) {
			return c.runner
		}
	}
	return nil
}

// frameworkOfDir resolves dir's framework, a worktree through its parent site,
// nil when it has none.
func frameworkOfDir(dir string) *Framework {
	name := ""
	if site, err := FindSiteByPath(dir); err == nil {
		name = site.Framework
	} else if parent, ok := ParentSiteForWorktreeDir(dir); ok {
		name = parent.Framework
	}
	if name == "" {
		name, _ = DetectFrameworkForDir(dir)
	}
	if name == "" {
		return nil
	}
	fw, ok := GetFrameworkForDir(name, dir)
	if !ok {
		return nil
	}
	return fw
}
