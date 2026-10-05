package cli

import (
	"maps"
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

	options, selected, _ := addSuggestedServices([]string{"redis", "mailpit"}, []string{"mailpit"}, fw, map[string]bool{"mysql": true}, false, allPresetsAvailable, nothingInstalled)

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

	options, selected, _ := addSuggestedServices(nil, []string{"mailpit"}, fw, nil, true, allPresetsAvailable, nothingInstalled)

	if !slices.Equal(options, []string{"solr"}) || !slices.Equal(selected, []string{"mailpit"}) {
		t.Errorf("options %v selected %v", options, selected)
	}
}

// A name the store does not publish cannot be installed, so it is not offered.
func TestAddSuggestedServicesSkipsUnknownPreset(t *testing.T) {
	fw := &config.Framework{SuggestServices: sugg("nosuch"), PackageServices: sugg("nosuch")}

	options, selected, _ := addSuggestedServices(nil, nil, fw, nil, false, func(string) bool { return false }, nothingInstalled)

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
	options, selected, _ := addSuggestedServices(nil, nil, fw, nil, false, allPresetsAvailable, func(n string) bool { return n == "valkey" })
	if !slices.Equal(options, []string{"valkey"}) || !slices.Equal(selected, []string{"valkey"}) {
		t.Errorf("options %v selected %v", options, selected)
	}
}

// Each offered suggestion keeps the package behind it, so the wizards can say
// why it is there; a service the framework and a package both suggest is
// explained by the package.
func TestAddSuggestedServicesNamesThePackage(t *testing.T) {
	fw := &config.Framework{
		SuggestServices: []config.ServiceSuggestion{{Name: "solr", Reason: "Search backend"}, {Name: "mercure", Reason: "Realtime"}},
		PackageServices: []config.ServiceSuggestion{{Name: "solr", Reason: "Search API backend", Package: "drupal/search_api_solr"}},
	}

	_, _, offered := addSuggestedServices([]string{"redis"}, nil, fw, nil, false, allPresetsAvailable, nothingInstalled)

	want := []config.ServiceSuggestion{
		{Name: "solr", Reason: "Search API backend", Package: "drupal/search_api_solr"},
		{Name: "mercure", Reason: "Realtime"},
	}
	if !slices.Equal(offered, want) {
		t.Errorf("offered = %v, want %v", offered, want)
	}
}

// A service the project's env already selected, Redis from a stock Laravel
// .env.example, still names the package that wants it.
func TestAddSuggestedServicesNamesThePackageOfAPreselectedService(t *testing.T) {
	fw := &config.Framework{
		PackageServices: []config.ServiceSuggestion{{Name: "redis", Reason: "Redis server for predis", Package: "predis/predis"}},
	}

	_, selected, offered := addSuggestedServices([]string{"redis"}, []string{"redis"}, fw, nil, false, allPresetsAvailable, nothingInstalled)

	want := []config.ServiceSuggestion{{Name: "redis", Reason: "Redis server for predis", Package: "predis/predis"}}
	if !slices.Equal(offered, want) {
		t.Errorf("offered = %v, want %v", offered, want)
	}
	if !slices.Equal(selected, []string{"redis"}) {
		t.Errorf("selected = %v, want redis once", selected)
	}
}

// The terminal form names the package beside a suggested service, falls back
// to the reason, and leaves every other service bare.
func TestServiceOptions(t *testing.T) {
	offered := []config.ServiceSuggestion{
		{Name: "redis", Reason: "Redis server for predis", Package: "predis/predis"},
		{Name: "mercure", Reason: "Realtime updates"},
	}
	got := map[string]string{}
	for _, o := range serviceOptions([]string{"mailpit", "redis", "mercure"}, offered) {
		got[o.Value] = o.Key
	}
	want := map[string]string{"mailpit": "mailpit", "redis": "redis (predis/predis)", "mercure": "mercure (Realtime updates)"}
	if !maps.Equal(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}
