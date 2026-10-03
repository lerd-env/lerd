//go:build !windows

package cli

import "errors"

// runWorkerExec only exists on Windows: the Linux worker units are supervised
// by systemd and the macOS ones run a guard script.
func runWorkerExec(_ string, _ []string) (int, error) {
	return 0, errors.New("lerd worker-exec runs on Windows only")
}
