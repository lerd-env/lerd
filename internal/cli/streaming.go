package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/screenshare"
)

// NewStreamingCmd returns `lerd streaming`, which hides the workspaces marked
// private, and their sites, from the dashboard and the TUI while screen sharing.
func NewStreamingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "streaming",
		Short: "Hide private workspaces and their sites while screen sharing",
	}
	cmd.AddCommand(onOffCmd("enable", "Turn streaming mode on as a feature, following screen shares on Linux Wayland", func() error { return setStreamingEnabled(true) }))
	cmd.AddCommand(onOffCmd("disable", "Turn streaming mode off as a feature and show everything", func() error { return setStreamingEnabled(false) }))
	cmd.AddCommand(onOffCmd("on", "Hide private workspaces and their sites", func() error { return setStreaming(true) }))
	cmd.AddCommand(onOffCmd("off", "Show private workspaces and their sites again", func() error { return setStreaming(false) }))
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
	if cfg, _ := config.LoadGlobal(); on && (cfg == nil || !cfg.UI.StreamingEnabled) {
		return fmt.Errorf("streaming mode is disabled, run lerd streaming enable first")
	}
	if err := SetStreamingMode(on); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("streaming mode " + onOff(on))
	return nil
}

func setStreamingEnabled(on bool) error {
	if err := config.SetStreamingEnabled(on); err != nil {
		return err
	}
	notifyWorkspaceChange()
	feedback.Begin()
	msg := "streaming mode disabled"
	if on {
		msg = "streaming mode enabled"
	}
	if on && !screenshare.Supported() {
		msg += ", switch it with lerd streaming on/off (no PipeWire, so shares are not detected)"
	}
	feedback.Done(msg)
	return nil
}
