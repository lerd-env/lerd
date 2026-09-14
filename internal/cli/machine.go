package cli

import "github.com/spf13/cobra"

// NewMachineCmd returns the machine parent command for Podman Machine
// maintenance operations.
func NewMachineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machine",
		Short: "Manage the Podman Machine VM (macOS)",
	}
	cmd.AddCommand(newMachineResetCmd())
	cmd.AddCommand(newMachineReclaimCmd())
	return cmd
}

func newMachineReclaimCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reclaim",
		Short: "Return disk the Podman Machine VM no longer uses to the host (macOS)",
		Long: `Return disk the Podman Machine VM is holding but no longer using.

The VM's disk image is a sparse file that only ever grows: blocks freed inside
the VM stay charged to the host until the guest filesystem is trimmed. This
caps the VM's systemd journal, vacuums it, then trims the filesystem, and
reports how much host disk came back. Containers, images and site data are
untouched.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMachineReclaim()
		},
	}
}

func newMachineResetCmd() *cobra.Command {
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Recreate the Podman Machine VM (fixes corrupted container storage; data is preserved)",
		Long: `Recreate the Podman Machine VM.

Use this when lerd start fails with a container-storage error such as
"getting graph driver info ... overlay: invalid argument" after an unclean
host shutdown. lerd's databases and site data are stored on the host and are
preserved; container images are rebuilt automatically on the next start.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMachineReset(assumeYes)
		},
	}
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}
