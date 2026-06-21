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

func runWhatsnew(_ *cobra.Command, _ []string) error {
	latest, err := lerdUpdate.FetchLatestVersion()
	if err != nil {
		return fmt.Errorf("could not fetch latest version: %w", err)
	}

	current := lerdUpdate.StripV(version.Version)
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
	if changelog == "" {
		fmt.Println("No changelog entries found.")
		return nil
	}
	for _, line := range strings.Split(changelog, "\n") {
		fmt.Println(line)
	}
	return nil
}
