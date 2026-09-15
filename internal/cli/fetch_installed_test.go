package cli

import (
	"reflect"
	"testing"
)

// `lerd fetch 7.4` builds the image and used to report the version ready, while
// php:list and `lerd new` both still called it missing: an image is not a
// runtime until php:rebuild writes the quadlet. The fetch has to say so, or the
// next command contradicts it.
func TestPHPVersionsWithoutRuntime(t *testing.T) {
	cases := []struct {
		name      string
		requested []string
		installed []string
		want      []string
	}{
		{
			name:      "reports the versions that have no runtime",
			requested: []string{"7.4", "8.1", "8.5"},
			installed: []string{"8.0", "8.5"},
			want:      []string{"7.4", "8.1"},
		},
		{
			name:      "silent when every requested version is installed",
			requested: []string{"8.4", "8.5"},
			installed: []string{"8.4", "8.5"},
			want:      nil,
		},
		{
			name:      "silent with nothing requested",
			requested: nil,
			installed: []string{"8.5"},
			want:      nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := phpVersionsWithoutRuntime(c.requested, c.installed)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("phpVersionsWithoutRuntime(%v, %v) = %v, want %v", c.requested, c.installed, got, c.want)
			}
		})
	}
}
