package siteinfo

import (
	"slices"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// A linked site is offered what its packages suggest, minus what it already
// uses and what the user turned down for it, with the reason kept. A package
// whose alternative the site already runs, valkey for drupal/redis, is quiet.
func TestSuggestedServices(t *testing.T) {
	none := func(string) bool { return false }
	pkg := []config.ServiceSuggestion{
		{Name: "redis", Package: "predis/predis"},
		{Name: "solr", Reason: "Search backend", Package: "drupal/search_api_solr"},
		{Name: "selenium", Package: "laravel/dusk"},
		{Name: "redis", Package: "drupal/redis"},
		{Name: "valkey", Package: "drupal/redis"},
	}
	got := suggestedServices(pkg, []string{"mysql", "valkey"}, []string{"selenium"}, none)
	want := []config.ServiceSuggestion{
		{Name: "redis", Package: "predis/predis"},
		{Name: "solr", Reason: "Search backend", Package: "drupal/search_api_solr"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := suggestedServices(nil, []string{"mysql"}, nil, none); got != nil {
		t.Errorf("no package suggestions should give nil, got %v", got)
	}
}
