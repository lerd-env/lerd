package node

// Linux daemons get a PATH that already covers where package managers put
// tools, so none of the macOS Homebrew lookups apply here.

// misePrefixes are the package-manager dirs a daemon's restricted PATH misses.
func misePrefixes() []string { return []string{"/usr/local/bin", "/usr/bin"} }

func brewBunDirs() []string      { return nil }
func osUnitPathDirs() []string   { return nil }
func osBrewNvmScripts() []string { return nil }
