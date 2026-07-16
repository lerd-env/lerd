package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/phpsets"
)

// printPerVersionStatus renders what each built version's image really carries.
//
// The declared set is an intention, not a fact: an image built before an entry
// was declared does not have it, and some versions cannot build it at all.
// Printing the declared set alone under an "every PHP version" heading
// advertised what those images do not load, which is what #856 exists to
// prevent. The three states are kept apart because the answer to each differs:
// rebuild, give up, or nothing to do.
func printPerVersionStatus(cfg *config.GlobalConfig, pick func(phpsets.Report) phpsets.SetState) {
	reports := phpsets.StatusAll(cfg, config.SupportedPHPVersions)

	var lines []string
	unbuildable := false
	for _, r := range reports {
		if !r.Built {
			continue // nothing known, so nothing claimed
		}
		if r.NeedsRebuild {
			lines = append(lines, fmt.Sprintf("  PHP %-4s image predates this set, run 'lerd php:rebuild %s'", r.Version, r.Version))
			continue
		}
		set := pick(r)
		switch {
		case len(set.Cannot) > 0 && len(set.Has) == 0:
			unbuildable = true
			lines = append(lines, fmt.Sprintf("  PHP %-4s cannot load: %s", r.Version, strings.Join(set.Cannot, ", ")))
		case len(set.Cannot) > 0:
			unbuildable = true
			lines = append(lines, fmt.Sprintf("  PHP %-4s %s (cannot load: %s)", r.Version, strings.Join(set.Has, ", "), strings.Join(set.Cannot, ", ")))
		default:
			lines = append(lines, fmt.Sprintf("  PHP %-4s %s", r.Version, strings.Join(set.Has, ", ")))
		}
	}
	if len(lines) == 0 {
		return
	}

	fmt.Println("\nPer version:")
	for _, l := range lines {
		fmt.Println(l)
	}
	if unbuildable {
		fmt.Println("\nWhat an image cannot load did not build on that version; a rebuild will not change that.")
	}
}

// extensionsOf and packagesOf name which declared set a report line is about.
func extensionsOf(r phpsets.Report) phpsets.SetState { return r.Extensions }
func packagesOf(r phpsets.Report) phpsets.SetState   { return r.Packages }
