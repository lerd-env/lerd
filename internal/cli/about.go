package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/version"
	"github.com/spf13/cobra"
)

// NewAboutCmd returns the about command.
func NewAboutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "about",
		Short: "Show information about Lerd",
		RunE:  runAbout,
	}
}

func runAbout(_ *cobra.Command, _ []string) error {
	fmt.Println("Lerd Oracle Edition — Podman-powered local PHP dev environment with baked-in Oracle Database support")
	fmt.Println()
	fmt.Printf("  Version  %s\n", version.Version)
	fmt.Printf("  Commit   %s\n", version.Commit)
	fmt.Printf("  Built    %s\n", version.Date)
	fmt.Println()
	fmt.Println("  Fork     https://github.com/gabriel-sousa99/lerd (Oracle Instant Client 21.18 + oci8)")
	fmt.Println("  Upstream https://github.com/geodro/lerd")
	fmt.Println()
	fmt.Println("  Oracle additions by Gabriel Sousa — built on lerd by George Dumitrescu")
	return nil
}
