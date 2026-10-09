//go:build !windows

package cli

import "errors"

// runWorkerExec only exists on Windows: the Linux worker units are supervised
// by systemd and the macOS ones run a guard script.
func runWorkerExec(_, _ string, _ bool, _ []string) (int, error) {
	return 0, errors.New("lerd worker-exec runs on Windows only")
}
