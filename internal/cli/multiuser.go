package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/systemd"
)

// NewMultiUserCmd returns `lerd multi-user`, which lets several users of one
// Linux machine take turns: each user's lerd runs only while their desktop is
// on screen.
func NewMultiUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "multi-user",
		Short: "Share this Linux machine with other lerd users, one at a time",
	}
	cmd.AddCommand(onOffCmd("enable", "Stop lerd while another user's desktop is on screen and start it again when yours is back", func() error { return setMultiUser(true) }))
	cmd.AddCommand(onOffCmd("disable", "Keep lerd running whoever is on screen", func() error { return setMultiUser(false) }))
	return cmd
}

func setMultiUser(on bool) error {
	if err := config.SetMultiUser(on); err != nil {
		return err
	}
	feedback.Begin()
	if !on {
		feedback.Done("multi-user mode disabled")
		return nil
	}
	feedback.Done("multi-user mode enabled, every user of this machine needs it on")
	return nil
}

// ErrOtherUserOnScreen is lerd start refusing because another user's desktop
// has the screen in multi-user mode; their stack owns the ports until ours is back.
var ErrOtherUserOnScreen = errors.New("another user's desktop is on screen and multi-user mode is on, so lerd starts again when yours is back; run lerd multi-user disable to start it anyway")

func seatRefusesStart(multiUser bool, fg systemd.Foreground) error {
	if !multiUser || fg != systemd.ForegroundOther {
		return nil
	}
	return ErrOtherUserOnScreen
}
