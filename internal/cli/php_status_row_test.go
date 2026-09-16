package cli

import "testing"

// A version nobody serves from has no container running, and that is idle, not
// broken. Painting it red told the reader to repair something lerd deliberately
// never started, and contradicted `lerd fetch`, which only builds the versions
// sites actually use.
func TestPHPVersionRowState(t *testing.T) {
	cases := []struct {
		name                       string
		imageExists, running, used bool
		want                       phpRowState
	}{
		{"serving", true, true, true, phpRowOK},
		{"built but unused", true, false, false, phpRowIdle},
		{"needed and down", true, false, true, phpRowDown},
		{"needed and unbuilt", false, false, true, phpRowImageMissing},
		{"unused and unbuilt", false, false, false, phpRowNotBuilt},
		{"running though unused", true, true, false, phpRowOK},
	}
	for _, c := range cases {
		if got := phpVersionRowState(c.imageExists, c.running, c.used); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
