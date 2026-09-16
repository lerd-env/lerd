package cli

import (
	"fmt"
	"strings"

	"github.com/geodro/lerd/internal/feedback"
	lerdUpdate "github.com/geodro/lerd/internal/update"
	"github.com/geodro/lerd/internal/version"
	"github.com/spf13/cobra"
)

// NewWhatsnewCmd returns the whatsnew command.
func NewWhatsnewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whatsnew",
		Short: "Show what changed in the latest release",
		RunE:  runWhatsnew,
	}
}

// whatsnewLatestStable and whatsnewLatestBeta are the two release lookups, as
// seams so the channel choice is testable without reaching GitHub.
var (
	whatsnewLatestStable = lerdUpdate.FetchLatestVersion
	whatsnewLatestBeta   = lerdUpdate.FetchLatestPrerelease
)

// whatsnewLatestFor resolves the version whatsnew should compare against, by the
// same rule status and update follow: a build that is itself a beta tracks the
// beta line, everything else tracks stable. Asking only for the latest stable
// made whatsnew deny an update that status was offering on the same machine.
func whatsnewLatestFor(current string) (string, error) {
	stable, err := whatsnewLatestStable()
	if err != nil {
		return "", err
	}
	if !lerdUpdate.FollowsBetas(lerdUpdate.StripGitDescribe(lerdUpdate.StripV(current))) {
		return stable, nil
	}
	pre, err := whatsnewLatestBeta()
	if err != nil {
		return stable, nil
	}
	if lerdUpdate.VersionGreaterThan(lerdUpdate.StripV(pre), lerdUpdate.StripV(stable)) {
		return pre, nil
	}
	return stable, nil
}

func runWhatsnew(_ *cobra.Command, _ []string) error {
	latest, err := whatsnewLatestFor(version.Version)
	if err != nil {
		return fmt.Errorf("could not fetch latest version: %w", err)
	}

	current := lerdUpdate.StripGitDescribe(lerdUpdate.StripV(version.Version))
	latestStripped := lerdUpdate.StripV(latest)

	if !lerdUpdate.VersionGreaterThan(latestStripped, current) {
		feedback.Begin()
		feedback.Done("you are on the latest version (" + version.Version + ")")
		return nil
	}

	changelog, err := lerdUpdate.FetchChangelog(current, latestStripped)
	if err != nil {
		return fmt.Errorf("could not fetch changelog: %w", err)
	}

	fmt.Printf("What's new in %s (you have %s):\n\n", latest, version.Version)
	summary := lerdUpdate.SummarizeChangelog(changelog)
	if summary == "" {
		fmt.Println("No changelog entries found.")
		return nil
	}
	for _, line := range strings.Split(summary, "\n") {
		fmt.Println(line)
	}
	return nil
}
