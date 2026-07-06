package config

import (
	"encoding/json"
	"testing"
)

// The store index is JSON with snake_case keys. FrameworkRule needs json tags,
// not just yaml, or "composer_sections" and "version_key" silently fail to bind
// (Go's case-insensitive fallback does not cross underscores), which drops
// Symfony's flex-require / extra.symfony.require version detection.
func TestFrameworkRule_JSONSnakeCaseTags(t *testing.T) {
	data := []byte(`{"composer":"symfony/framework-bundle","composer_sections":["flex-require"],"version_key":"extra.symfony.require"}`)
	var r FrameworkRule
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.VersionKey != "extra.symfony.require" {
		t.Errorf("version_key not bound: %q", r.VersionKey)
	}
	if len(r.ComposerSections) != 1 || r.ComposerSections[0] != "flex-require" {
		t.Errorf("composer_sections not bound: %+v", r.ComposerSections)
	}
}
