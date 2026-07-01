// Package presetfixtures embeds the add-on service presets that ship in the
// external lerd-env/services store rather than the binary, and exposes them as
// an fs.FS. Tests wire it under the config preset seam (via
// config.SetExtraPresetsForTest) so mechanism and functionality tests keep
// resolving add-ons — dependencies, families, dashboards, auto-login file
// mounts — without a network fetch. It is imported only from _test.go files, so
// nothing here reaches the production binary. It imports no lerd packages, so any
// package's tests can use it without an import cycle.
package presetfixtures

import (
	"embed"
	"io/fs"
)

//go:embed testdata/*.yaml
var embedded embed.FS

// FS returns the add-on presets as a flat filesystem of <name>.yaml files.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "testdata")
	if err != nil {
		panic(err)
	}
	return sub
}
