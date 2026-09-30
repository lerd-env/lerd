package podman

import (
	"os"
	"os/user"
	"slices"

	"github.com/geodro/lerd/internal/config"
)

// homeAliases returns the spellings of home that the unit's own %h mount does
// not cover. Each is mounted once at its own path, so a command run from any of
// them finds its folder in the container.
func homeAliases(spellings []string, unitHome string) []string {
	var out []string
	for _, s := range spellings {
		if s != unitHome && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// unitHomeDir is what systemd expands %h to: the home directory in the user
// database, which on an ostree host need not be the spelling $HOME carries.
var unitHomeDir = func() string {
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	h, _ := os.UserHomeDir()
	return h
}

// homeAliasMounts is homeAliases for this machine. Only a %h that names the
// same folder as home counts, so a HOME pointed somewhere else entirely, as
// tests and sudo do, never mounts it.
func homeAliasMounts(home string) []string {
	unitHome := unitHomeDir()
	if unitHome == "" || !config.SamePath(home, unitHome) {
		return nil
	}
	return homeAliases(config.HomeSpellings(home), unitHome)
}

// unitMissingHomeAliases reports whether a running container lacks one of the
// home alias mounts. It asks by destination, since a source check resolves the
// alias onto the %h mount and never sees it missing.
func unitMissingHomeAliases(unit string, aliases []string) bool {
	if len(aliases) == 0 {
		return false
	}
	running, destinations := containerMountField(unit, "Destination")
	if !running {
		return false
	}
	for _, a := range aliases {
		if !slices.Contains(destinations, a) {
			return true
		}
	}
	return false
}
