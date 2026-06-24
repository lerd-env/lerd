package cli

import (
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/siteinfo"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// NewSitesCmd returns the sites command.
func NewSitesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sites",
		Short: "List all registered sites",
		RunE:  runSites,
	}
}

func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 120 // assume wide if not a tty
	}
	return w
}

func runSites(_ *cobra.Command, _ []string) error {
	sites, err := siteinfo.LoadAll(siteinfo.EnrichCLI)
	if err != nil {
		return err
	}

	if len(sites) == 0 {
		fmt.Println("No sites registered. Use 'lerd park' or 'lerd link' to add sites.")
		return nil
	}

	width := termWidth()

	// Order sites: each main/standalone followed by its group secondaries (a
	// secondary occupies <label>.<main-domain> and reads as a child of the main,
	// like aliases and worktrees do).
	secondariesByGroup := map[string][]siteinfo.EnrichedSite{}
	for _, s := range sites {
		if s.Group != "" && s.GroupSubdomain != "" {
			secondariesByGroup[s.Group] = append(secondariesByGroup[s.Group], s)
		}
	}
	var ordered []orderedSite
	seen := map[string]bool{}
	for _, s := range sites {
		if s.GroupSubdomain != "" {
			continue // printed under its main
		}
		ordered = append(ordered, orderedSite{s, false})
		seen[s.Name] = true
		for _, sec := range secondariesByGroup[s.Group] {
			ordered = append(ordered, orderedSite{sec, true})
			seen[sec.Name] = true
		}
	}
	// Any secondary whose main isn't listed (shouldn't happen, but never hide a site).
	for _, s := range sites {
		if s.GroupSubdomain != "" && !seen[s.Name] {
			ordered = append(ordered, orderedSite{s, true})
		}
	}

	// Narrow terminals keep the two-line compact layout; a bordered grid can't
	// fit that width. Wider terminals get the styled table.
	if width < 80 {
		for _, o := range ordered {
			printSiteCompact(o.site, o.grouped)
		}
		return nil
	}

	wide := width >= 120
	headers := []string{"Domain", "PHP", "TLS", "Framework", "Status", "Path"}
	if wide {
		headers = []string{"Name", "Domain", "PHP", "Node", "TLS", "Framework", "Status", "Path"}
	}
	var rows [][]string
	for _, o := range ordered {
		rows = append(rows, siteRows(o.site, o.grouped, wide)...)
	}
	feedback.Table(headers, rows)
	return nil
}

type orderedSite struct {
	site    siteinfo.EnrichedSite
	grouped bool
}

func pausedTag() string { return feedback.Amber("paused") }

// siteRows renders a site as one main row plus a child row per alias domain and
// worktree, the nested rows indented inside the first column so the grid keeps
// its parent/child shape. wide adds the Name and Node columns (≥120 cols);
// otherwise the domain leads (80–119 cols).
func siteRows(s siteinfo.EnrichedSite, grouped, wide bool) [][]string {
	tls := "No"
	if s.Secured {
		tls = "Yes"
	}
	status := ""
	if s.Paused {
		status = pausedTag()
	}

	var rows [][]string
	if wide {
		name := truncate(s.Name, 22)
		if grouped {
			name = "↳ grp " + truncate(s.Name, 14)
		}
		rows = append(rows, []string{name, truncate(s.PrimaryDomain(), 32), s.PHPVersion, s.NodeVersion, tls, s.FrameworkLabel, status, s.Path})
		for _, d := range s.Domains[1:] {
			rows = append(rows, []string{"↳ alias", truncate(d, 32), "", "", "", "", "", ""})
		}
		for _, wt := range s.Worktrees {
			rows = append(rows, []string{"↳ " + truncate(wt.Branch, 18), truncate(wt.Domain, 32), s.PHPVersion, s.NodeVersion, "—", "", "", wt.Path})
		}
		return rows
	}

	dom := truncate(s.PrimaryDomain(), 28)
	if grouped {
		dom = "↳ grp " + truncate(s.PrimaryDomain(), 20)
	}
	rows = append(rows, []string{dom, s.PHPVersion, tls, s.FrameworkLabel, status, s.Path})
	for _, d := range s.Domains[1:] {
		rows = append(rows, []string{"↳ " + truncate(d, 24), "", "", "", "", ""})
	}
	for _, wt := range s.Worktrees {
		rows = append(rows, []string{"↳ " + truncate(wt.Domain, 24), s.PHPVersion, "—", "", "", wt.Path})
	}
	return rows
}

// printSiteCompact prints a site as two indented lines (plus its aliases and
// worktrees) for terminals too narrow for the table. grouped renders a group
// secondary nested beneath its main.
func printSiteCompact(s siteinfo.EnrichedSite, grouped bool) {
	status := ""
	if s.Paused {
		status = " [" + pausedTag() + "]"
	}
	tls := ""
	if s.Secured {
		tls = " 🔒"
	}
	meta := s.PHPVersion
	if s.FrameworkLabel != "" {
		meta += " · " + s.FrameworkLabel
	}

	if grouped {
		fmt.Printf("  ↳ grp %s%s%s\n", s.PrimaryDomain(), tls, status)
		fmt.Printf("    %s\n", truncate(s.Path, 74))
		if meta != "" {
			fmt.Printf("    %s\n", feedback.Dim(meta))
		}
	} else {
		fmt.Printf("%s%s%s\n", s.PrimaryDomain(), tls, status)
		fmt.Printf("  %s\n", truncate(s.Path, 76))
		if meta != "" {
			fmt.Printf("  %s\n", feedback.Dim(meta))
		}
	}

	for _, d := range s.Domains[1:] {
		fmt.Printf("  ↳ %s\n", d)
	}
	for _, wt := range s.Worktrees {
		fmt.Printf("  ↳ %s\n", wt.Domain)
		fmt.Printf("    %s\n", truncate(wt.Path, 74))
	}
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-3]) + "..."
}
