//go:build !windows

package main

import "os"

// disableQuickEdit is Windows-only; the package builds elsewhere so it is
// vetted and tested with the rest of lerd.
func disableQuickEdit() {}

func redirectStderr(*os.File) {}

func claimAgent() bool { return true }
