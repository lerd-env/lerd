package cli

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geodro/lerd/internal/man"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewManCmd returns the man command.
func NewManCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "man [page]",
		Short: "Browse the Lerd documentation",
		Long: "Browse and search the Lerd documentation in the terminal.\n\n" +
			"Run without arguments to open the interactive browser, or pass a page\n" +
			"name to jump directly (e.g. lerd man sites, lerd man usage/sites).",
		RunE: runMan,
	}
}

func runMan(_ *cobra.Command, args []string) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return runManPlain(args)
	}

	// Detect glamour style now, before bubbletea enters alt screen and takes
	// over stdin — glamour's auto-detection sends an OSC terminal query that
	// would otherwise block until a key press.
	glamStyle := glamourStyle()

	pages := man.BuildRegistry()
	m := man.NewModel(pages, args, glamStyle)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// glamourStyle returns the glamour style to use for markdown rendering.
// Reads GLAMOUR_STYLE env var; falls back to "dark".
func glamourStyle() string {
	if s := os.Getenv("GLAMOUR_STYLE"); s != "" {
		return s
	}
	return "dark"
}

func runManPlain(args []string) error {
	pages := man.BuildRegistry()

	if len(args) > 0 {
		query := args[0]
		for _, p := range pages {
			if p.Slug == query || p.Section+"/"+p.Slug == query {
				fmt.Print(p.Content())
				return nil
			}
		}
		return fmt.Errorf("no documentation page found for %q", query)
	}

	// Print table of contents
	fmt.Println("Lerd Documentation")
	fmt.Println(strings.Repeat("─", 40))
	lastSection := "SENTINEL"
	for _, p := range pages {
		if p.Section != lastSection {
			lastSection = p.Section
			fmt.Printf("\n%s\n", man.SectionLabel(p.Section))
		}
		fmt.Printf("  %-32s  lerd man %s\n", p.Title, p.Slug)
	}
	return nil
}
