package config

import (
	"slices"
	"testing"
)

func has(names ...string) func(string) bool {
	return func(n string) bool { return slices.Contains(names, n) }
}

// A package lists its services most important first and puts one forward: the
// first installed here, else its first. Nothing when the project already uses
// any of them, and a service two packages both put forward appears once.
func TestPickPackageSuggestions(t *testing.T) {
	all := []ServiceSuggestion{
		{Name: "redis", Package: "predis/predis"},
		{Name: "valkey", Package: "predis/predis"},
		{Name: "elasticsearch", Package: "drupal/elasticsearch_connector"},
		{Name: "opensearch", Package: "drupal/elasticsearch_connector"},
		{Name: "redis", Package: "laravel/horizon"},
		{Name: "valkey", Package: "laravel/horizon"},
		{Name: "solr", Package: "drupal/search_api_solr"},
	}

	names := func(got []ServiceSuggestion) []string {
		var out []string
		for _, sg := range got {
			out = append(out, sg.Name)
		}
		return out
	}

	got := PickPackageSuggestions(all, has(), has("valkey", "opensearch"))
	if want := []string{"valkey", "opensearch", "solr"}; !slices.Equal(names(got), want) {
		t.Errorf("installed first: got %v, want %v", names(got), want)
	}

	got = PickPackageSuggestions(all, has(), has())
	if want := []string{"redis", "elasticsearch", "solr"}; !slices.Equal(names(got), want) {
		t.Errorf("nothing installed: got %v, want %v", names(got), want)
	}

	got = PickPackageSuggestions(all, has("valkey", "solr"), has())
	if want := []string{"elasticsearch"}; !slices.Equal(names(got), want) {
		t.Errorf("project already uses one: got %v, want %v", names(got), want)
	}
}
