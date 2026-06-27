package cli

import "testing"

func TestHasSubIDRange(t *testing.T) {
	const content = "alice:100000:65536\n# comment\n\nbob:165536:65536\n1001:231072:65536\n"
	tests := []struct {
		name          string
		username, uid string
		want          bool
	}{
		{"by username", "alice", "1000", true},
		{"second entry", "bob", "1000", true},
		{"by uid", "carol", "1001", true},
		{"missing", "dave", "9999", false},
		{"empty username falls back to uid", "", "1001", true},
		{"comment line not matched", "#", "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasSubIDRange(content, tt.username, tt.uid); got != tt.want {
				t.Errorf("hasSubIDRange(%q, %q): got %v, want %v", tt.username, tt.uid, got, tt.want)
			}
		})
	}
}

func TestHasSubIDRange_empty(t *testing.T) {
	if hasSubIDRange("", "alice", "1000") {
		t.Error("empty content should yield no range")
	}
}
