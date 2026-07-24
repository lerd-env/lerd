package linker

import "testing"

func TestHostProxyGate(t *testing.T) {
	cases := []struct {
		name                                 string
		command                              string
		disabled, skipConfirm, approved, tty bool
		wantProceed, wantPrompt              bool
	}{
		{"disabled blocks even when approved", "npm run dev", true, false, true, true, false, false},
		{"proxy-only empty command proceeds", "", false, false, false, false, true, false},
		{"disabled still allows proxy-only", "", true, false, false, false, true, false},
		{"already approved proceeds", "npm run dev", false, false, true, false, true, false},
		{"skip_confirmation proceeds", "npm run dev", false, true, false, false, true, false},
		{"interactive unapproved needs prompt", "npm run dev", false, false, false, true, false, true},
		{"non-interactive unapproved is refused", "npm run dev", false, false, false, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proceed, prompt, reason := HostProxyGate(c.command, c.disabled, c.skipConfirm, c.approved, c.tty)
			if proceed != c.wantProceed || prompt != c.wantPrompt {
				t.Errorf("HostProxyGate = (proceed %v, prompt %v), want (%v, %v)", proceed, prompt, c.wantProceed, c.wantPrompt)
			}
			if !proceed && !prompt && reason == "" {
				t.Errorf("a refusal must carry a reason")
			}
		})
	}
}
