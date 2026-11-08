package applog

import "testing"

// A site's log is whatever the app wrote; parsing it must never panic and must
// respect the entry cap the log viewer asks for.
func FuzzParseLaravel(f *testing.F) {
	f.Add("[2026-11-08 10:00:00] local.ERROR: boom {\"exception\":\"x\"}\n#0 /app/x.php(1)\n", 10)
	f.Add("[2026-11-08 10:00:00] production.INFO: ok\n[2026-11-08 10:00:01] local.WARNING: w\n", 1)
	f.Add("no header\n\n[", 0)

	f.Fuzz(func(t *testing.T, content string, max int) {
		if max < 0 || max > 1000 {
			t.Skip()
		}
		entries := parseLaravel(content, max)
		if max > 0 && len(entries) > max {
			t.Fatalf("asked for at most %d entries, got %d", max, len(entries))
		}
	})
}
