package cli

import (
	"strings"
	"testing"
)

func TestNewMultiSelectShowsEveryOption(t *testing.T) {
	cases := []struct {
		name        string
		description string
		options     []string
	}{
		{name: "one option", options: []string{"schedule"}},
		{name: "two options", options: []string{"queue", "schedule"}},
		{name: "with description", description: "Auto-start when linking", options: []string{"queue", "schedule"}},
		{name: "six options", description: "Deselect to remove", options: []string{"mailpit", "meilisearch", "redis", "rustfs", "gotenberg", "opensearch"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var selected []string
			view := newMultiSelect("Workers", tc.description, tc.options, &selected).View()
			for _, opt := range tc.options {
				if !strings.Contains(view, opt) {
					t.Errorf("option %q missing from rendered field:\n%s", opt, view)
				}
			}
		})
	}
}

func TestNewMultiSelectKeepsTitleAndDescription(t *testing.T) {
	var selected []string
	view := newMultiSelect("Workers", "Auto-start when linking", []string{"queue"}, &selected).View()

	if !strings.Contains(view, "Workers") {
		t.Errorf("title missing from rendered field:\n%s", view)
	}
	if !strings.Contains(view, "Auto-start when linking") {
		t.Errorf("description missing from rendered field:\n%s", view)
	}
}

func TestNewMultiSelectBindsValue(t *testing.T) {
	selected := []string{"schedule"}
	field := newMultiSelect("Workers", "", []string{"queue", "schedule"}, &selected)

	got, ok := field.GetValue().([]string)
	if !ok {
		t.Fatalf("GetValue returned %T, want []string", field.GetValue())
	}
	if len(got) != 1 || got[0] != "schedule" {
		t.Errorf("bound value = %v, want [schedule]", got)
	}
}
