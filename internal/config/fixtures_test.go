package config

import "github.com/geodro/lerd/internal/presetfixtures"

// Add-ons ship in the external store, not the binary. Tests resolve them through
// the seam's extra layer so mechanism and functionality coverage (dependencies,
// families, dashboard auto-login file mounts) survives without a network fetch.
func init() { SetExtraPresetsForTest(presetfixtures.FS()) }
