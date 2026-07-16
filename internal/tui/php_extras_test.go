package tui

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// The System view rebuilds every frame, so this summary reads only config: what
// a version's image loaded is recorded when it is built, and asking podman here
// would shell out on every render.
func TestPHPExtrasSummary(t *testing.T) {
	cfg := &config.GlobalConfig{}

	// Nothing declared is the common case, and it must add no row at all
	// rather than an empty one.
	if got := phpExtrasSummary(cfg, "8.4"); got != "" {
		t.Errorf("phpExtrasSummary with nothing declared = %q, want empty", got)
	}

	cfg.AddExtension("mongodb")
	cfg.AddPackage("chromium")

	// A version with no record has never been built since these were declared.
	if got := phpExtrasSummary(cfg, "8.1"); got != "not in this image yet · lerd php:rebuild 8.1" {
		t.Errorf("unrecorded version = %q, want a rebuild hint", got)
	}

	// A version carrying everything reports just what it has.
	cfg.SetRealised("8.5", config.RealisedPHPSet{Extensions: []string{"mongodb"}, Packages: []string{"chromium"}})
	if got := phpExtrasSummary(cfg, "8.5"); got != "mongodb, chromium" {
		t.Errorf("complete version = %q, want the realised set", got)
	}

	// A version missing part of the set says so without claiming why: the TUI
	// cannot tell "did not build" from "image predates it" without reading an
	// image label, so it states only what is true either way.
	cfg.SetRealised("7.4", config.RealisedPHPSet{Packages: []string{"chromium"}})
	if got := phpExtrasSummary(cfg, "7.4"); got != "chromium · not in this image: mongodb" {
		t.Errorf("partial version = %q, want the gap named without a cause", got)
	}
}
