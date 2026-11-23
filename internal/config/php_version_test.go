package config

import "testing"

func TestValidPHPVersion(t *testing.T) {
	for _, s := range []string{"8.3", "8.4", "8.5", "10.0", "7.4"} {
		if !ValidPHPVersion(s) {
			t.Errorf("ValidPHPVersion(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "8", "8,5", "8.", ".5", "eight.five", "8.5.1", "8.x", " 8.5", "../8.5", "8.5/..", "8.5\n"} {
		if ValidPHPVersion(s) {
			t.Errorf("ValidPHPVersion(%q) = true, want false", s)
		}
	}
}
