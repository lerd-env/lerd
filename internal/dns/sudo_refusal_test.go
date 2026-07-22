package dns

import "testing"

// The probe cannot read the root-only drop-in, so it decides whether the grant
// is gone from how sudo words its refusal. Classic sudo and sudo-rs word it
// differently, and matching only the classic wording made every sudo-rs refusal
// read as inconclusive, so a deleted drop-in was never rewritten.
func TestSudoRefusalIsConclusive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name:   "classic sudo",
			stderr: "sudo: a password is required\n",
			want:   true,
		},
		{
			name:   "sudo-rs, shipped on Ubuntu 26.04",
			stderr: "sudo: interactive authentication is required\n",
			want:   true,
		},
		{
			name:   "mixed case is still a refusal",
			stderr: "sudo: A Password Is Required\n",
			want:   true,
		},
		{
			name:   "no sudo on the host says nothing about the grant",
			stderr: "sudo: command not found\n",
			want:   false,
		},
		{
			name:   "an unsupported flag is not evidence either way",
			stderr: "sudo: invalid option -- 'n'\n",
			want:   false,
		},
		{
			name:   "silence is inconclusive",
			stderr: "",
			want:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sudoRefusalIsConclusive(tc.stderr); got != tc.want {
				t.Errorf("sudoRefusalIsConclusive(%q) = %v, want %v", tc.stderr, got, tc.want)
			}
		})
	}
}
