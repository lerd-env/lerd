//go:build !linux

package cli

import "fmt"

func runBootstrapSystem(string, bool) error {
	return fmt.Errorf("lerd bootstrap is only supported on Linux")
}

func runBootstrapTrustCA(string, string) error {
	return fmt.Errorf("lerd bootstrap is only supported on Linux")
}

func runBootstrapUntrustCA() error {
	return fmt.Errorf("lerd bootstrap is only supported on Linux")
}
