package tui

import (
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// phpExtrasSummary describes what a PHP version's image carries of the declared
// extension/package set, for one System row.
//
// Config only, deliberately: systemRows rebuilds on every frame, so reading an
// image label or running php -m here would shell out to podman on every render.
// The realised record is written when the image is built, so it is already the
// answer.
//
// It never says why something is absent. Telling "did not build on this
// version" apart from "this image predates the set" needs the image label, and
// claiming the wrong one is worse than naming the gap plainly. The CLI and the
// dashboard, which can afford the podman call, draw that distinction.
//
// Returns "" when nothing is declared, so the common case adds no row.
func phpExtrasSummary(cfg *config.GlobalConfig, version string) string {
	if cfg == nil {
		return ""
	}
	declared := append(slices.Clone(cfg.GetExtensions()), cfg.GetPackages()...)
	if len(declared) == 0 {
		return ""
	}

	realised := cfg.GetRealised(version)
	have := append(slices.Clone(realised.Extensions), realised.Packages...)
	if len(have) == 0 {
		return "not in this image yet · lerd php:rebuild " + version
	}

	summary := strings.Join(have, ", ")
	if missing := cfg.MissingFromImage(version, declared); len(missing) > 0 {
		summary += " · not in this image: " + strings.Join(missing, ", ")
	}
	return summary
}
