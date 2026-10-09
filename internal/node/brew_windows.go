package node

// Windows has no Homebrew and no daemon PATH gaps to fill.

func misePrefixes() []string     { return nil }
func brewBunDirs() []string      { return nil }
func osUnitPathDirs() []string   { return nil }
func osBrewNvmScripts() []string { return nil }

// needsExecBit is false: Windows has no execute bits, so a regular file is enough.
const needsExecBit = false
