//go:build !windows

package main

// disableQuickEdit is Windows-only; the package builds elsewhere so it is
// vetted and tested with the rest of lerd.
func disableQuickEdit() {}
