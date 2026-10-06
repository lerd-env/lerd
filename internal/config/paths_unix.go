//go:build !windows

package config

// osBaseDir has no OS default to offer: the home-relative XDG layout applies.
func osBaseDir(string) (string, bool) { return "", false }
