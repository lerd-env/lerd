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
func printPerVersionStatus(cfg *config.GlobalConfig, kind declaredKind) {
	pick := kind.pick
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
	// Landing on no version at all is a different problem from a version
	// boundary: nowhere to build is usually the build environment, not the
	// versions, and saying so is what would have surfaced #1013 without
	// anyone reading a build log.
	if nowhere := phpsets.NowhereBuilt(reports, pick); len(nowhere) > 0 {
		fmt.Printf("\n%s built on no version at all, which usually means the build failed rather than\nthe versions cannot have it.%s\n", strings.Join(nowhere, ", "), kind.nowhereHint(nowhere[0]))
	}
}

// declaredKind names which declared set a report is about, along with what to
// suggest when an entry of that kind landed on no version.
type declaredKind struct {
	pick        func(phpsets.Report) phpsets.SetState
	nowhereHint func(entry string) string
}

var extensionsOf = declaredKind{
	pick: func(r phpsets.Report) phpsets.SetState { return r.Extensions },
	nowhereHint: func(e string) string {
		return " Re-add it with the Alpine packages its\nbuild needs: lerd php:ext add " + e + " --apk-deps \"<pkg>-dev\""
	},
}

var packagesOf = declaredKind{
	pick:        func(r phpsets.Report) phpsets.SetState { return r.Packages },
	nowhereHint: func(e string) string { return " Check the package name exists on Alpine." },
}
