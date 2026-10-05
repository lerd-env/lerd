package cli

import "errors"

// packagedPrefixes is empty: /usr/local is an ordinary install prefix on macOS
// (and where Intel Homebrew lives), with no package manager to hand the job to.
var packagedPrefixes []string

func rollbackSupported() error {
	return errors.New("rollback is not supported on macOS — use 'brew switch lerd <version>' instead")
}
