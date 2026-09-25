package siteops

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	projectNameJunk = regexp.MustCompile(`[^a-z0-9.-]+`)
	projectNameRuns = regexp.MustCompile(`-{2,}`)
)

// ProjectSlug turns what someone typed as a new project's name into one that
// works as a directory, a site name and a DNS label all at once: lowercase,
// with anything but letters, digits, dots and dashes collapsed into one dash.
func ProjectSlug(s string) string {
	s = projectNameJunk.ReplaceAllString(strings.ToLower(s), "-")
	s = projectNameRuns.ReplaceAllString(s, "-")
	return strings.Trim(s, "-.")
}

// CheckProjectName refuses a new project's name that ProjectSlug would change,
// naming the one it would use, so every way of creating a project agrees on
// what a name may be.
func CheckProjectName(name string) error {
	slug := ProjectSlug(name)
	if slug == "" {
		return fmt.Errorf("enter a name for the project, using lowercase letters, digits, dots and dashes")
	}
	if slug != name {
		return fmt.Errorf("project name %q can only use lowercase letters, digits, dots and dashes; try %q", name, slug)
	}
	return nil
}
