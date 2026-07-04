package ui

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/presetfixtures"
)

// Add-ons ship in the external store, not the binary. Tests resolve them through
// the config seam's extra layer so add-on-shaped mechanism and functionality
// tests keep running without a network fetch.
func init() { config.SetExtraPresetsForTest(presetfixtures.FS()) }
