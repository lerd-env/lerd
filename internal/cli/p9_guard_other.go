//go:build !windows

package cli

import "errors"

// runP9Guard only exists on Windows, the one host lerd serves 9p from.
func runP9Guard(_ string, _ []string) error {
	return errors.New("lerd p9-guard runs on Windows only")
}
