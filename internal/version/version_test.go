package version

import "testing"

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{
		"1.38.0":                    true,
		"1.38.0-beta.1":             true,
		"":                          false,
		"dev":                       false,
		"1.37.0-12-g9d514e93":       false,
		"1.37.0-12-g9d514e93-dirty": false,
	} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}
