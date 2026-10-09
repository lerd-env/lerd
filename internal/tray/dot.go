//go:build !nogui

package tray

import "strings"

// statusDots are the coloured status markers menu titles lead with.
var statusDots = []string{"🟢", "🔴", "🟡", "⚪"}

// splitDot separates a leading status dot from the rest of a menu title.
func splitDot(title string) (dot, rest string) {
	trimmed := strings.TrimLeft(title, " ")
	for _, d := range statusDots {
		if strings.HasPrefix(trimmed, d) {
			return d, strings.TrimLeft(strings.TrimPrefix(trimmed, d), " ")
		}
	}
	return "", title
}
