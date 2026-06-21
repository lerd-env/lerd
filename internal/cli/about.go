package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/feedback"
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
	feedback.Begin()
	fmt.Println("  " + feedback.Title("lerd"))
	fmt.Println("  " + feedback.Dim("Podman-powered local PHP development for Linux & macOS"))
	feedback.NewSummary().
		Row("Version", feedback.Val(version.Version)).
		Row("Commit", version.Commit).
		Row("Built", version.Date).
		Row("Repo", feedback.Val("https://github.com/geodro/lerd")).
		Print()
	feedback.Begin()
	fmt.Println("  " + feedback.Dim("© George Dumitrescu"))
	return nil
}
