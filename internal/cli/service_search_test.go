package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Searching used to reach only the external store, so anything already installed
// came back as "No matching presets in the service store" — which reads as "no
// such service" for one that is running.
func TestMatchPresets(t *testing.T) {
	presets := []config.PresetMeta{
		{Name: "redis", Description: "Redis in-memory store"},
		{Name: "mailpit", Description: "Mail catcher"},
		{Name: "meilisearch", Description: "Search engine", Category: "search-engine"},
	}

	names := func(in []config.PresetMeta) []string {
		out := make([]string, 0, len(in))
		for _, p := range in {
			out = append(out, p.Name)
		}
		return out
	}

	if got := names(matchPresets(presets, "mailpit")); !sameStrings(got, []string{"mailpit"}) {
		t.Errorf("installed preset by name = %v, want [mailpit]", got)
	}
	if got := names(matchPresets(presets, "search-engine")); !sameStrings(got, []string{"meilisearch"}) {
		t.Errorf("by category = %v, want [meilisearch]", got)
	}
	if got := names(matchPresets(presets, "STORE")); !sameStrings(got, []string{"redis"}) {
		t.Errorf("by description, case-insensitive = %v, want [redis]", got)
	}
	if got := names(matchPresets(presets, "")); len(got) != 3 {
		t.Errorf("empty query = %v, want all three", got)
	}
	if got := names(matchPresets(presets, "nothing-here")); len(got) != 0 {
		t.Errorf("no match = %v, want empty", got)
	}
}
