//go:build !linux

package cli

import "fmt"

func runBootstrapSystem(string) error {
	return fmt.Errorf("lerd bootstrap is only supported on Linux")
}

func runBootstrapTrustCA(string) error {
	return fmt.Errorf("lerd bootstrap is only supported on Linux")
}
