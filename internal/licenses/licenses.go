// Package licenses carries the third-party notices that ship inside the lerd
// binaries. Regenerate the embedded file with `make licenses`.
package licenses

import _ "embed"

//go:embed THIRD-PARTY-LICENSES.md
var notices string

// Notices returns the third-party license notices as markdown.
func Notices() string { return notices }
