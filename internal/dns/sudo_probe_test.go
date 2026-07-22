package dns

import (
	"slices"
	"strings"
	"testing"
)

// `sudo -l CMD` answers whether CMD is permitted, not whether it can run without
// a password, so it exits 0 for any user carrying a broader rule, which is most
// admin accounts. The probe has to actually run the command to learn the answer.
func TestSudoProbeCmd_RunsTheCommandRatherThanListingIt(t *testing.T) {
	args := sudoProbeCmd().Args

	if slices.Contains(args, "-l") {
		t.Errorf("probe must not use `sudo -l`, which reports authorization rather than passwordless access: %v", args)
	}
	if !slices.Contains(args, "-n") {
		t.Errorf("probe must stay non-interactive so it can never hang on a prompt: %v", args)
	}
	if !slices.Contains(args, sudoersProbeCommand) {
		t.Errorf("probe must invoke the granted command %q: %v", sudoersProbeCommand, args)
	}
}

// sudo translates its refusal, so the parser only ever sees English if the probe
// pins the locale. A Romanian host reports "o parolă este necesară".
func TestCLocaleEnv_PinsTheLocale(t *testing.T) {
	got := cLocaleEnv([]string{
		"PATH=/usr/bin",
		"LANG=ro_RO.UTF-8",
		"LC_ALL=ro_RO.UTF-8",
		"LC_MESSAGES=ro_RO.UTF-8",
		"LANGUAGE=ro",
		"HOME=/home/george",
	})

	// glibc's getenv returns the first match, so a stale entry left earlier in
	// the slice would win over one appended after it.
	for _, kv := range got {
		for _, stale := range []string{"LANG=ro", "LC_ALL=ro", "LC_MESSAGES=ro", "LANGUAGE=ro"} {
			if strings.HasPrefix(kv, stale) {
				t.Errorf("inherited locale %q must be dropped, not shadowed", kv)
			}
		}
	}
	if !slices.Contains(got, "LC_ALL=C") {
		t.Errorf("probe environment must pin LC_ALL=C, got %v", got)
	}
	for _, want := range []string{"PATH=/usr/bin", "HOME=/home/george"} {
		if !slices.Contains(got, want) {
			t.Errorf("unrelated variable %q must be preserved, got %v", want, got)
		}
	}
}
