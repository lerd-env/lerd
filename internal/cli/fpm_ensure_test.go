package cli

import (
	"bufio"
	"strings"
	"testing"
)

func TestChooseInstalledVersion(t *testing.T) {
	versions := []string{"8.2", "8.3", "8.4"}
	cases := []struct {
		in   string
		want string
	}{
		{"1\n", "8.2"},    // by index
		{"3\n", "8.4"},    // last index
		{"8.3\n", "8.3"},  // by literal version
		{"\n", ""},        // blank cancels
		{"0\n", ""},       // out of range
		{"9\n", ""},       // out of range
		{"abc\n", ""},     // garbage
		{"  2 \n", "8.3"}, // surrounding space trimmed
	}
	for _, c := range cases {
		r := bufio.NewReader(strings.NewReader(c.in))
		if got := chooseInstalledVersion(r, versions); got != c.want {
			t.Errorf("chooseInstalledVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReadConfirmAnswerReader(t *testing.T) {
	cases := []struct {
		in         string
		defaultYes bool
		want       bool
	}{
		{"\n", true, true},   // blank → default yes
		{"\n", false, false}, // blank → default no
		{"y\n", false, true}, // explicit yes overrides default
		{"yes\n", false, true},
		{"n\n", true, false}, // explicit no overrides default
		{"no\n", true, false},
		{"NO\n", true, false}, // case-insensitive
	}
	for _, c := range cases {
		r := bufio.NewReader(strings.NewReader(c.in))
		if got := readConfirmAnswerReader(r, "Install it?", c.defaultYes); got != c.want {
			t.Errorf("readConfirmAnswerReader(%q, default=%v) = %v, want %v", c.in, c.defaultYes, got, c.want)
		}
	}
}
