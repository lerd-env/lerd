//go:build windows

package cli

import (
	"strings"
	"testing"
)

// Windows rarely sets HOME, and every path it has is a host path. The exec
// environment has to carry the profile dir and composer's home as the paths
// the container reaches them at, or composer looks for its cache at C:/...
func TestContainerExecEnvArgsGivesContainerPathsOnWindows(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", `C:\Users\me`)
	t.Setenv("COMPOSER_HOME", `C:\Users\me\.composer`)

	got := strings.Join(containerExecEnvArgs(`C:\Users\me\Sites\app`), " ")
	for _, want := range []string{
		"HOME=/mnt/c/Users/me ",
		"COMPOSER_HOME=/mnt/c/Users/me/.composer ",
		"PATH=/mnt/c/Users/me/Sites/app/vendor/bin:",
		":/mnt/c/Users/me/.composer/vendor/bin",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("env args %q missing %q", got, want)
		}
	}
	if strings.Contains(got, `C:\`) || strings.Contains(got, "C:/") {
		t.Errorf("a host path reached the container env: %q", got)
	}
}
