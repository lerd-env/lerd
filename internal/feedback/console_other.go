//go:build !windows

package feedback

// enableANSI is a no-op off Windows: Unix terminals render ANSI natively.
func enableANSI() bool { return true }
