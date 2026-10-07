//go:build !darwin

package config

// macosAccentIndex has no macOS to read off a Mac.
func macosAccentIndex() string { return "" }
