package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpPortsCmd returns the php:ports parent command, which manages extra host
// ports published on a PHP version's shared FPM container so a process bound
// inside `lerd shell` is reachable at localhost:PORT. The list is per version
// and independent; a host port already claimed is shifted to the next free one.
func NewPhpPortsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "php:ports",
		Short: "Publish extra host ports on a PHP version's shell container",
	}
	cmd.AddCommand(newPhpPortsAddCmd())
	cmd.AddCommand(newPhpPortsRemoveCmd())
	cmd.AddCommand(newPhpPortsListCmd())
	return cmd
}

func newPhpPortsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <host:container...>",
		Short: "Publish a host:container port and restart the version's FPM",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flagVer, _ := cmd.Flags().GetString("php")
			version, err := phpPkgVersion(flagVer)
			if err != nil {
				return err
			}
			feedback.Begin()
			for _, spec := range args {
				host, container, perr := parsePortArg(spec)
				if perr != nil {
					return perr
				}
				actual, aerr := podman.AddFPMPort(version, host, container)
				if aerr != nil {
					return aerr
				}
				if actual != host {
					feedback.Note(fmt.Sprintf("%d in use, published on %d", host, actual))
				}
				feedback.Line(fmt.Sprintf("PHP %s: localhost:%d -> container %d", version, actual, container))
			}
			feedback.Done("shell ports updated for PHP " + version)
			return nil
		},
	}
	cmd.Flags().String("php", "", "PHP version (defaults to the current project or global default)")
	return cmd
}

func newPhpPortsRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <host...>",
		Short: "Unpublish a host port and restart the version's FPM",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flagVer, _ := cmd.Flags().GetString("php")
			version, err := phpPkgVersion(flagVer)
			if err != nil {
				return err
			}
			feedback.Begin()
			for _, a := range args {
				host, perr := strconv.Atoi(strings.TrimSpace(a))
				if perr != nil {
					return fmt.Errorf("invalid host port %q", a)
				}
				if err := podman.RemoveFPMPort(version, host); err != nil {
					return err
				}
				feedback.Line(fmt.Sprintf("PHP %s: unpublished %d", version, host))
			}
			feedback.Done("shell ports updated for PHP " + version)
			return nil
		},
	}
	cmd.Flags().String("php", "", "PHP version (defaults to the current project or global default)")
	return cmd
}

func newPhpPortsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List extra host ports published for a PHP version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flagVer, _ := cmd.Flags().GetString("php")
			version, err := phpPkgVersion(flagVer)
			if err != nil {
				return err
			}
			ports := config.FPMPortsFor(version)
			if len(ports) == 0 {
				fmt.Printf("No shell ports published for PHP %s.\n", version)
				return nil
			}
			fmt.Printf("Shell ports for PHP %s:\n", version)
			for _, p := range ports {
				fmt.Printf("  - %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().String("php", "", "PHP version (defaults to the current project or global default)")
	return cmd
}

// parsePortArg splits a "host:container" argument into its two ports. A bare
// number is taken as both host and container (publish the same port straight
// through), the common shell-server case (a Vite server on 5173 reached at 5173).
func parsePortArg(spec string) (host, container int, err error) {
	spec = strings.TrimSpace(spec)
	if !strings.Contains(spec, ":") {
		n, cerr := strconv.Atoi(spec)
		if cerr != nil {
			return 0, 0, fmt.Errorf("invalid port %q", spec)
		}
		return n, n, nil
	}
	parts := strings.SplitN(spec, ":", 2)
	host, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid host port in %q", spec)
	}
	container, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid container port in %q", spec)
	}
	return host, container, nil
}
