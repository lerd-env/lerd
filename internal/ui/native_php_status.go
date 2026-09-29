package ui

import (
	"context"
	"runtime"

	"github.com/geodro/lerd/internal/nativephp"
	"github.com/geodro/lerd/internal/tools"
)

// nativePHPStatus reports the patch a version's host build sits at and whether
// a newer one has been published.
//
// A build with no recorded patch predates lerd stamping them. It is installed
// and serving, so it is reported as it is rather than as an update waiting to
// happen, which would put an update badge on every version after an upgrade.
func nativePHPStatus(installed, pinned, publishedAt, installedPublished string) (patch string, update bool) {
	if installed == "" || pinned == "" {
		return installed, false
	}
	return installed, tools.BuildIsStale(pinned, installed, publishedAt, installedPublished)
}

// nativePHPStatusFor reads a version's host build state from disk and the pins.
func nativePHPStatusFor(m *tools.Manifest, version string) (string, bool) {
	name := nativephp.ToolName(version)
	return nativePHPStatus(
		tools.InstalledVersion(name),
		m.Tools[name].Version,
		m.PublishedAt(name, runtime.GOOS, runtime.GOARCH),
		tools.InstalledPublished(name),
	)
}

// nativePins loads the published pins once per status build.
func nativePins() *tools.Manifest { return tools.Load(context.Background()) }
