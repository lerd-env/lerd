package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/screenshare"
)

// NewStreamingCmd returns `lerd streaming`, which hides the sites and workspaces
// marked private from the dashboard and the TUI while screen sharing.
func NewStreamingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "streaming",
		Short: "Hide private sites and workspaces while screen sharing",
	}
	cmd.AddCommand(onOffCmd("on", "Hide private sites and workspaces", func() error { return setStreaming(true) }))
	cmd.AddCommand(onOffCmd("off", "Show private sites and workspaces again", func() error { return setStreaming(false) }))

	auto := &cobra.Command{
		Use:   "auto",
		Short: "Turn streaming mode on by itself while the screen is shared (Linux Wayland)",
	}
	auto.AddCommand(onOffCmd("on", "Follow screen shares", func() error { return setStreamingAuto(true) }))
	auto.AddCommand(onOffCmd("off", "Stop following screen shares", func() error { return setStreamingAuto(false) }))
	cmd.AddCommand(auto)
	return cmd
}

func onOffCmd(verb, short string, run func() error) *cobra.Command {
	return &cobra.Command{
		Use:   verb,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return run() },
	}
}

// SetStreamingMode saves the mode and nudges a running dashboard to redraw.
// Turning it on first tells open dashboards to hide private sites at once,
// since the full redraw that follows takes a noticeable moment.
func SetStreamingMode(on bool) error {
	if on {
		_, _, _ = postUnix("/api/internal/streaming-on", nil)
	}
	if err := config.SetStreamingMode(on); err != nil {
		return err
	}
	notifyWorkspaceChange()
	return nil
}

func setStreaming(on bool) error {
	if err := SetStreamingMode(on); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("streaming mode " + onOff(on))
	return nil
}

func setStreamingAuto(on bool) error {
	if on && !screenshare.Supported() {
		return fmt.Errorf("screen share detection needs PipeWire (pw-dump), which this host does not have")
	}
	if err := config.SetStreamingAuto(on); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("streaming mode follows screen shares: " + onOff(on))
	return nil
}
