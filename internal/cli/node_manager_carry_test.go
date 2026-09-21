package cli

import (
	"reflect"
	"testing"
)

// A switch must leave every version the old manager could run still runnable,
// so the majors the new one is missing are the ones to install.
func TestMissingMajors(t *testing.T) {
	cases := []struct {
		name     string
		from, to []string
		want     []string
	}{
		{"nothing carried when the new manager has them all", []string{"22", "20"}, []string{"20", "22"}, nil},
		{"the gap is carried in the old manager's order", []string{"24", "22", "20", "18"}, []string{"22"}, []string{"24", "20", "18"}},
		{"a fresh manager takes everything", []string{"22"}, nil, []string{"22"}},
		{"an empty old manager asks for nothing", nil, []string{"22"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingMajors(tc.from, tc.to); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("missingMajors(%v, %v) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}
