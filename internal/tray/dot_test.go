//go:build !nogui

package tray

import "testing"

func TestSplitDot(t *testing.T) {
	cases := []struct{ in, dot, rest string }{
		{"  🟢 nginx", "🟢", "nginx"},
		{"🔴 Stopped", "🔴", "Stopped"},
		{"  ⚪ dns (disabled)", "⚪", "dns (disabled)"},
		{"… 3 more", "", "… 3 more"},
		{"✔ 8.3", "", "✔ 8.3"},
	}
	for _, c := range cases {
		if d, r := splitDot(c.in); d != c.dot || r != c.rest {
			t.Errorf("splitDot(%q) = %q,%q want %q,%q", c.in, d, r, c.dot, c.rest)
		}
	}
}
