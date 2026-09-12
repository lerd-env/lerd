package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	lerdUpdate "github.com/geodro/lerd/internal/update"
	"github.com/spf13/cobra"
)

// NewUpdateBetaCmd returns the update:beta command, which opts a stable install
// into the prerelease line. With no argument it reports where the install sits.
func NewUpdateBetaCmd(currentVersion string) *cobra.Command {
	return &cobra.Command{
		Use:       "update:beta [on|off]",
		Short:     "Offer beta releases to a stable install",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return reportBetaChannel(currentVersion)
			}
			return setBetaChannel(args[0] == "on")
		},
	}
}

func setBetaChannel(on bool) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	cfg.SetBetaChannel(on)
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	feedback.Begin()
	if on {
		feedback.Done("beta updates on — lerd update will offer prereleases as they are published")
		return nil
	}
	feedback.Done("beta updates off — only stable releases from here")
	return nil
}

func reportBetaChannel(currentVersion string) error {
	feedback.Begin()
	cfg, _ := config.LoadGlobal()
	switch {
	case cfg.IsBetaChannel():
		feedback.Note("beta updates are " + feedback.Val("on"))
	case lerdUpdate.FollowsBetas(currentVersion):
		// Running a beta already follows the line without the flag, so saying
		// "off" here would read as a promise of stable-only updates.
		feedback.Note("beta updates are " + feedback.Val("off") + ", but this build is a beta and follows the beta line")
	default:
		feedback.Note("beta updates are " + feedback.Val("off"))
	}
	return nil
}
