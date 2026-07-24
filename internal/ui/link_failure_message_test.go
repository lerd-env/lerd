package ui

import (
	"strings"
	"testing"
)

func TestFailureMessage(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{
			// What `lerd env` actually prints for a project with no framework:
			// the reason on the ✗ line, then the guidance under it. Taking only
			// the last line dropped the reason and kept the guidance.
			name: "a failure marker carries the reason and its guidance",
			out: " ✗ no framework detected for this site\n" +
				"Define one with 'lerd framework add' or add a framework YAML to /home/me/.config/lerd/frameworks\n",
			want: "no framework detected for this site — Define one with 'lerd framework add' " +
				"or add a framework YAML to /home/me/.config/lerd/frameworks",
		},
		{
			name: "progress output above the failure is left out",
			out:  " → configuring .env…\n → detecting services…\n ✗ reading .env: permission denied\n",
			want: "reading .env: permission denied",
		},
		{
			name: "without a marker the last line is the best available",
			out:  "some output\nfinal problem\n",
			want: "final problem",
		},
		{
			name: "a single line with no marker is used as is",
			out:  "exit status 1",
			want: "exit status 1",
		},
		{
			name: "empty output yields nothing rather than a stray separator",
			out:  "\n\n",
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := failureMessage(c.out); got != c.want {
				t.Errorf("failureMessage() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFailureMessage_trimsAWallOfOutput(t *testing.T) {
	got := failureMessage(" ✗ " + strings.Repeat("x", 500))
	if len([]rune(got)) > 301 {
		t.Errorf("message is %d runes, want it trimmed", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a trimmed message must say so, got %q", got[len(got)-10:])
	}
}
