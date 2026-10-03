//go:build !windows

package p9share

import "fmt"

// Serve is Windows-only: only a Hyper-V machine mounts its folders over hvsock.
func Serve([]Share, int) error {
	return fmt.Errorf("p9-serve only runs on Windows")
}
