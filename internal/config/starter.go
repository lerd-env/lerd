package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Starter is another way to start a project of a framework the store already
// defines: a different skeleton to create from, packages to require into the new
// project, or both. It only applies at creation. The project is detected as its
// framework afterwards, so whatever a required package needs to finish its
// install is that package's own setup step, offered by lerd setup as usual.
type Starter struct {
	// Starter is the name the file is published under, starters/<framework>/<name>.yaml.
	Starter     string `yaml:"starter"`
	Label       string `yaml:"label"`
	Description string `yaml:"description,omitempty"`
	// Framework is the framework the starter builds on and the range of its
	// majors the starter supports.
	Framework FrameworkPackageScope `yaml:"framework"`
	// Create replaces the framework's own create command, for a starter that is
	// a different skeleton. The target directory is appended as for a framework.
	Create string `yaml:"create,omitempty"`
	// Require lists composer packages, optionally with a constraint after a
	// colon, to require into the new project.
	Require []string `yaml:"require,omitempty"`
}

// StoreStarterEntry is one starter the store index lists, enough of its file for
// the create step to offer it without fetching every starter first.
type StoreStarterEntry struct {
	Name        string                `json:"name"`
	Label       string                `json:"label"`
	Description string                `json:"description,omitempty"`
	Framework   FrameworkPackageScope `json:"framework"`
}

var (
	starterName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// requireConstraint is what composer require accepts after the colon. It is
	// one argv entry, so the only thing to keep out is a space splitting it.
	requireConstraint = regexp.MustCompile(`^\S+$`)
)

// ValidStarterName reports whether name can be a starter's file name.
func ValidStarterName(name string) bool {
	return starterName.MatchString(name)
}

// Supports reports whether the starter can start a project on this major.
func (s *Starter) Supports(version string) bool {
	return s.Framework.covers(version)
}

// Supports reports whether the listed starter can start a project on this major.
func (e StoreStarterEntry) Supports(version string) bool {
	return e.Framework.covers(version)
}

// Validate refuses a starter lerd could not act on, naming what is wrong. Every
// require entry becomes an argument to composer, so one that would read as a
// flag or as two arguments is refused rather than passed on.
func (s *Starter) Validate() error {
	if !ValidStarterName(s.Starter) {
		return fmt.Errorf("starter name %q is not lowercase letters, digits and dashes", s.Starter)
	}
	if s.Label == "" {
		return fmt.Errorf("starter %s has no label", s.Starter)
	}
	if s.Framework.Name == "" {
		return fmt.Errorf("starter %s names no framework", s.Starter)
	}
	if s.Create != "" && strings.TrimSpace(s.Create) == "" {
		return fmt.Errorf("starter %s has an empty create command", s.Starter)
	}
	if s.Create == "" && len(s.Require) == 0 {
		return fmt.Errorf("starter %s neither creates from a skeleton nor requires a package", s.Starter)
	}
	for _, req := range s.Require {
		name, constraint, hasConstraint := strings.Cut(req, ":")
		if !composerPackageName.MatchString(name) || (hasConstraint && !requireConstraint.MatchString(constraint)) {
			return fmt.Errorf("starter %s requires %q, which is not a composer package with an optional :constraint", s.Starter, req)
		}
	}
	return nil
}

// RequireArgs is the composer command line, after the binary, that requires the
// starter's packages into the project at dir. A skeleton created without
// installing has no lock yet, and resolving one now would pin it to the PHP the
// scaffold ran under rather than the one the project is set up on, so the
// packages only go into composer.json for setup's install to resolve.
func (s *Starter) RequireArgs(dir string) []string {
	args := []string{"require", "--no-interaction"}
	if _, err := os.Stat(filepath.Join(dir, "composer.lock")); err != nil {
		args = append(args, "--no-update")
	}
	return append(args, s.Require...)
}

// StoreStarters returns the starters the cached store index lists for a
// framework, in the order the index lists them.
func StoreStarters(framework string) []StoreStarterEntry {
	idx := loadCachedStoreIndex()
	if idx == nil {
		return nil
	}
	var out []StoreStarterEntry
	for _, e := range idx.Starters {
		if e.Framework.Name == framework && ValidStarterName(e.Name) {
			out = append(out, e)
		}
	}
	return out
}
