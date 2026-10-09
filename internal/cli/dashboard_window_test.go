package cli

import "testing"

func TestIsDashboardWindowTitle(t *testing.T) {
	for title, want := range map[string]bool{
		"Lerd - Google Chrome":             true,
		"Dashboard · Lerd - Vivaldi":       true,
		"Sites · Lerd - Google Chrome":     true,
		"Lerd — Mozilla Firefox":           true,
		"Lerd - Personal - Microsoft Edge": true,
		"Lerd":                             false,
		"Lerd - Notepad":                   false,
		"Lerdy - Google Chrome":            false,
		"My Lerd notes - Google Chrome":    false,
		"phpMyAdmin - Google Chrome":       false,
	} {
		if got := isDashboardWindowTitle(title); got != want {
			t.Errorf("isDashboardWindowTitle(%q) = %v, want %v", title, got, want)
		}
	}
}
