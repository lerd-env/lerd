package ui

import (
	"context"

	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// nativePHPStatus reports the patch a version's host build sits at and whether
// a newer one has been published.
//
// A build with no recorded patch predates lerd stamping them. It is installed
// and serving, so it is reported as it is rather than as an update waiting to
// happen, which would put an update badge on every version after an upgrade.
func nativePHPStatus(installed, pinned string) (patch string, update bool) {
	if installed == "" || pinned == "" {
		return installed, false
	}
	return installed, installed != pinned
}

// nativePHPStatusFor reads a version's host build state from disk and the pins.
func nativePHPStatusFor(m *tools.Manifest, version string) (string, bool) {
	return nativePHPStatus(
		tools.InstalledVersion(nativephp.ToolName(version)),
		m.Tools[nativephp.ToolName(version)].Version,
	)
}

// nativePins loads the published pins once per status build.
func nativePins() *tools.Manifest { return tools.Load(context.Background()) }
