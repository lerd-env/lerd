package cli

import (
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func allPresetsAvailable(string) bool { return true }

func nothingInstalled(string) bool { return false }

// sugg builds suggestions, each from a package of its own.
func sugg(names ...string) []config.ServiceSuggestion {
	out := make([]config.ServiceSuggestion, len(names))
	for i, n := range names {
		out[i] = config.ServiceSuggestion{Name: n, Package: "vendor/" + n}
	}
	return out
}

// The framework's suggestions are offered unticked, a package's are ticked, and
// neither is listed twice or among the non-database services when it is a database.
func TestAddSuggestedServices(t *testing.T) {
	fw := &config.Framework{SuggestServices: sugg("solr", "redis", "mysql"), PackageServices: sugg("solr")}

	options, selected := addSuggestedServices([]string{"redis", "mailpit"}, []string{"mailpit"}, fw, map[string]bool{"mysql": true}, false, allPresetsAvailable, nothingInstalled)

	if !slices.Equal(options, []string{"redis", "mailpit", "solr"}) {
		t.Errorf("options = %v", options)
	}
	if !slices.Equal(selected, []string{"mailpit", "solr"}) {
		t.Errorf("selected = %v", selected)
	}
}

// A project that already saved its services keeps its answer: a package's
// suggestion is offered but not ticked over what the user chose.
func TestAddSuggestedServicesKeepsSavedAnswer(t *testing.T) {
	fw := &config.Framework{PackageServices: sugg("solr")}

	options, selected := addSuggestedServices(nil, []string{"mailpit"}, fw, nil, true, allPresetsAvailable, nothingInstalled)

	if !slices.Equal(options, []string{"solr"}) || !slices.Equal(selected, []string{"mailpit"}) {
		t.Errorf("options %v selected %v", options, selected)
	}
}

// A name the store does not publish cannot be installed, so it is not offered.
func TestAddSuggestedServicesSkipsUnknownPreset(t *testing.T) {
	fw := &config.Framework{SuggestServices: sugg("nosuch"), PackageServices: sugg("nosuch")}

	options, selected := addSuggestedServices(nil, nil, fw, nil, false, func(string) bool { return false }, nothingInstalled)

	if len(options) != 0 || len(selected) != 0 {
		t.Errorf("options %v selected %v", options, selected)
	}
}

// A package offering alternatives puts one forward: the one this machine runs.
func TestAddSuggestedServicesPicksInstalledAlternative(t *testing.T) {
	fw := &config.Framework{PackageServices: []config.ServiceSuggestion{
		{Name: "redis", Package: "predis/predis"},
		{Name: "valkey", Package: "predis/predis"},
	}}
	options, selected := addSuggestedServices(nil, nil, fw, nil, false, allPresetsAvailable, func(n string) bool { return n == "valkey" })
	if !slices.Equal(options, []string{"valkey"}) || !slices.Equal(selected, []string{"valkey"}) {
		t.Errorf("options %v selected %v", options, selected)
	}
}
