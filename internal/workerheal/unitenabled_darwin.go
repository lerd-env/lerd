package workerheal

// isUnitEnabled is Linux-only for now: macOS workers are launchd jobs with
// different enable semantics, so we treat enabled-state as unknown there and
// never flag a stopped worker as drift. Failed-state detection/healing is
// unaffected on macOS.
func isUnitEnabled(string) bool { return false }
