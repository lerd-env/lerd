package cli

// packagedPrefixes is empty: no package manager owns a Windows install path.
var packagedPrefixes []string

func rollbackSupported() error { return nil }
