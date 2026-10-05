// Package platform is where per-OS differences meet shared code. Shared files
// may read runtime.GOOS only as data; a branch on it belongs in a _linux.go,
// _darwin.go or _windows.go file. seams_test.go enforces that rule.
package platform
