package cli

import (
	"regexp"
	"strings"
)

var browserTitleSuffixes = []string{"Google Chrome", "Microsoft Edge", "Mozilla Firefox", "Brave", "Opera", "Vivaldi", "Chromium"}

// dashboardTitle matches the page title, "Lerd" or "<page> · Lerd", followed by
// the browser's own " - Vivaldi" style suffix.
var dashboardTitle = regexp.MustCompile(`(^|· )Lerd [-—] `)

// isDashboardWindowTitle reports whether a top-level window title is a browser
// showing the dashboard: "Dashboard · Lerd - Vivaldi", "Lerd — Mozilla Firefox".
func isDashboardWindowTitle(title string) bool {
	if !dashboardTitle.MatchString(title) {
		return false
	}
	for _, b := range browserTitleSuffixes {
		if strings.Contains(title, b) {
			return true
		}
	}
	return false
}
