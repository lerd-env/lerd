package tui

import (
	"reflect"
	"testing"
)

func TestNodePickerArgs(t *testing.T) {
	cases := []struct {
		name         string
		ver          string
		currentlyBun bool
		want         [][]string
	}{
		{"bun pins the runtime", "bun", false, [][]string{{"js:runtime", "bun"}}},
		{"node version while on bun clears the runtime first", "20", true,
			[][]string{{"js:runtime", "node"}, {"isolate:node", "20"}}},
		{"node version when not on bun just pins the version", "22", false,
			[][]string{{"isolate:node", "22"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nodePickerArgs(tc.ver, tc.currentlyBun)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("nodePickerArgs(%q, %v) = %v, want %v", tc.ver, tc.currentlyBun, got, tc.want)
			}
		})
	}
}

func TestFrankenPHPRunnable_FiltersUnpublishableVersions(t *testing.T) {
	installed := []string{"8.1", "8.2", "8.3", "8.4"}

	t.Run("frankenphp drops sub-8.2 versions", func(t *testing.T) {
		got := frankenPHPRunnable("frankenphp", installed, "8.3")
		want := []string{"8.2", "8.3", "8.4"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("frankenPHPRunnable = %v, want %v", got, want)
		}
	})

	t.Run("fpm site keeps every installed version", func(t *testing.T) {
		got := frankenPHPRunnable("fpm", installed, "8.1")
		if !reflect.DeepEqual(got, installed) {
			t.Errorf("non-frankenphp runtime should be untouched, got %v", got)
		}
	})

	t.Run("current version is kept even when unpublishable", func(t *testing.T) {
		got := frankenPHPRunnable("frankenphp", installed, "8.1")
		want := []string{"8.1", "8.2", "8.3", "8.4"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("current 8.1 should survive the filter, got %v", got)
		}
	})

	t.Run("empty result falls back to the full list", func(t *testing.T) {
		got := frankenPHPRunnable("frankenphp", []string{"7.4", "8.0"}, "")
		want := []string{"7.4", "8.0"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("filter that empties the list should fall back, got %v", got)
		}
	})
}
