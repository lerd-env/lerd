package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/sitedoctor"
	"github.com/spf13/cobra"
)

// NewSiteDoctorCmd returns the `site:doctor` command — framework-agnostic
// app-level health checks for a single site (distinct from `lerd doctor`, which
// diagnoses the lerd environment).
func NewSiteDoctorCmd() *cobra.Command {
	var asJSON, fix bool
	cmd := &cobra.Command{
		Use:          "site:doctor [domain]",
		Short:        "Run app-level health checks for a site",
		Long:         "Run app-level health checks (env, dependencies, security audit, framework specifics) for a site. Defaults to the site in the current directory; pass a domain to target another.",
		Example:      "  lerd site:doctor\n  lerd site:doctor acme.test\n  lerd site:doctor --json\n  lerd site:doctor --fix",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			domain := ""
			if len(args) == 1 {
				domain = args[0]
			}
			return runSiteDoctor(domain, asJSON, fix)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the report as JSON")
	cmd.Flags().BoolVar(&fix, "fix", false, "Apply the findings lerd can resolve on its own (a drifted nginx vhost, a stale worker unit), then re-check")
	return cmd
}

func runSiteDoctor(domain string, asJSON, fix bool) error {
	path, fwName, label, err := resolveSiteDoctorTarget(domain)
	if err != nil {
		return err
	}
	resp := sitedoctor.RunForPath(context.Background(), path, fwName)
	if fix {
		resp = applySiteDoctorFixes(path, fwName, resp, asJSON)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}
	printSiteDoctor(resp, label)
	if resp.Failures > 0 {
		os.Exit(1)
	}
	return nil
}

// readyDeclaredServices brings every service the site declares to running:
// installing the ones this machine never had and starting the ones that are
// merely stopped. It reports whether anything changed, so a run where every
// attempt failed does not claim to have fixed something, and it stops at the
// first failure rather than pulling several images to fail the same way.
func readyDeclaredServices(path string, fw *config.Framework, quiet bool) (bool, error) {
	changed := false
	for _, name := range sitedoctor.MissingDeclaredServices(path, fw) {
		if !quiet {
			fmt.Printf("  %s\n", feedback.Dim("installing "+name))
		}
		if _, err := serviceops.InstallPresetStreaming(name, "", func(serviceops.PhaseEvent) {}); err != nil {
			return changed, fmt.Errorf("installing %s: %w", name, err)
		}
		changed = true
		if !quiet {
			fmt.Printf("  %s\n\n", feedback.Dim(name+" is installed and running"))
		}
	}
	for _, name := range sitedoctor.StoppedDeclaredServices(path, fw) {
		if err := serviceops.StartService(name); err != nil {
			return changed, fmt.Errorf("starting %s: %w", name, err)
		}
		changed = true
		if !quiet {
			fmt.Printf("  %s\n\n", feedback.Dim("started "+name))
		}
	}
	return changed, nil
}

// createMissingDatabases creates the databases the project points at that their
// engine does not hold, which is what a missing schema needs before migrations
// have anywhere to run. It reports whether anything was created and stops at the
// first failure rather than working through an engine that just refused.
func createMissingDatabases(path string, quiet bool) (bool, error) {
	created := false
	for _, t := range sitedoctor.MissingDatabases(path) {
		if _, err := serviceops.CreateDatabase(t.Service, t.Database); err != nil {
			return created, fmt.Errorf("creating %s on %s: %w", t.Database, t.Service, err)
		}
		created = true
		// The re-check that follows runs inside the list cache's window, so
		// without this it reads the engine's contents from before the create.
		sitedoctor.ForgetDatabases(t.Service)
		if !quiet {
			fmt.Printf("  %s\n\n", feedback.Dim("created the "+t.Database+" database on "+t.Service))
		}
	}
	return created, nil
}

// createMissingBuckets creates the entities the site claims on a service that
// does not hold them. The create command comes from the service's own
// declaration, so nothing here knows what a bucket is.
func createMissingBuckets(path string, quiet bool) (bool, error) {
	created := false
	for _, t := range sitedoctor.MissingOwnedEntities(path) {
		spec := serviceops.EntityFor(t.Service, t.Kind)
		if spec == nil {
			continue
		}
		if err := serviceops.RunEntityAction(t.Service, spec, "create", t.Name); err != nil {
			return created, fmt.Errorf("creating %s on %s: %w", t.Name, t.Service, err)
		}
		created = true
		sitedoctor.ForgetEntities(t.Service)
		if !quiet {
			fmt.Printf("  %s\n\n", feedback.Dim("created the "+t.Name+" "+strings.ToLower(t.Label)+" on "+t.Service))
		}
	}
	return created, nil
}

// applySiteDoctorFixes resolves the findings lerd can act on by itself and
// returns a fresh report: a drifted vhost is rewritten, a database the engine
// does not hold is created, and a service picked but not wired has its
// connection written. The composer and npm ones are left out;
// they run in the site's container behind a run lock and stream their output,
// which belongs to the surfaces that can show it.
func applySiteDoctorFixes(path, fwName string, resp sitedoctor.Response, quiet bool) sitedoctor.Response {
	fixed := false
	for _, c := range resp.Checks {
		switch c.Fix {
		case sitedoctor.FixVhostRegenerate:
			if err := sitedoctor.FixVhost(path); err != nil {
				feedback.Warn("regenerating the vhost: %v", err)
				continue
			}
			if !quiet {
				fmt.Printf("  %s\n\n", feedback.Dim("regenerated the site's nginx vhost"))
			}
			fixed = true
		case sitedoctor.FixInstallServices, sitedoctor.FixStartServices:
			fw, _ := config.GetFrameworkForDir(fwName, path)
			ready, err := readyDeclaredServices(path, fw, quiet)
			if err != nil {
				feedback.Warn("%v", err)
			}
			if ready {
				fixed = true
			}
		case sitedoctor.FixStaleWorkers:
			site, err := config.FindSiteByPath(path)
			if err != nil || site == nil {
				continue
			}
			n, err := RemoveStaleWorkerUnits(*site)
			if err != nil {
				feedback.Warn("removing stale worker units: %v", err)
			}
			if n > 0 {
				if !quiet {
					fmt.Printf("  %s\n\n", feedback.Dim(fmt.Sprintf("removed %d stale worker unit(s)", n)))
				}
				fixed = true
			}
		case sitedoctor.FixCreateDatabase:
			created, err := createMissingDatabases(path, quiet)
			if err != nil {
				feedback.Warn("%v", err)
			}
			if created {
				fixed = true
			}
		case sitedoctor.FixCreateBucket:
			created, err := createMissingBuckets(path, quiet)
			if err != nil {
				feedback.Warn("%v", err)
			}
			if created {
				fixed = true
			}
		case sitedoctor.FixEnvSync:
			if err := runLerdEnvTo(path, fixOutput(quiet)); err != nil {
				feedback.Warn("writing the env: %v", err)
				continue
			}
			if !quiet {
				fmt.Printf("  %s\n\n", feedback.Dim("wrote the connection values for the services this project picks"))
			}
			fixed = true
		}
	}
	if !fixed {
		return resp
	}
	return sitedoctor.RunForPath(context.Background(), path, fwName)
}

// resolveSiteDoctorTarget returns the project path, framework name, and a label
// for the report. With a domain it resolves the registered site; without one it
// uses the current directory and detects the framework.
func resolveSiteDoctorTarget(domain string) (path, fwName, label string, err error) {
	if domain != "" {
		site, err := config.FindSiteByDomain(domain)
		if err != nil {
			return "", "", "", fmt.Errorf("site not found: %s", domain)
		}
		return site.Path, site.Framework, domain, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", "", err
	}
	// Leave fwName empty; sitedoctor.RunForPath detects it from the path.
	return cwd, "", cwd, nil
}

func printSiteDoctor(resp sitedoctor.Response, label string) {
	fmt.Printf("Doctor %s\n\n", feedback.Dim("· "+label))
	if len(resp.Checks) == 0 {
		fmt.Println(feedback.Dim("  no checks applied to this site"))
		return
	}
	for _, c := range resp.Checks {
		label := c.Label
		if label == "" {
			label = c.Name
		}
		fmt.Printf("  %s %s\n", doctorGlyph(c.Status), label)
		if c.Detail != "" {
			fmt.Printf("      %s\n", feedback.Dim(c.Detail))
		}
		if c.Fix != "" {
			fmt.Printf("      %s %s\n", feedback.Dim("fix:"), c.Fix)
		}
	}
	fmt.Println()
	switch {
	case resp.Failures > 0 || resp.Warnings > 0:
		fmt.Printf("  %s\n", feedback.Dim(fmt.Sprintf("%d failing · %d %s", resp.Failures, resp.Warnings,
			sitedoctor.Plural(resp.Warnings, "warning", "warnings"))))
	default:
		fmt.Printf("  %s\n", feedback.Green("all checks pass"))
	}
}

func doctorGlyph(status string) string {
	switch status {
	case sitedoctor.StatusOK:
		return feedback.Green(feedback.GlyphOK)
	case sitedoctor.StatusWarn:
		return feedback.Amber(feedback.GlyphWarn)
	case sitedoctor.StatusFail:
		return feedback.Red(feedback.GlyphFail)
	default:
		return feedback.Dim("?")
	}
}

// fixOutput is where a fix's subprocess writes. `--json` puts a document on
// stdout, so the child's prose goes to stderr instead: a caller parsing stdout
// gets JSON and nothing else, and a human still sees what happened.
func fixOutput(quiet bool) io.Writer {
	if quiet {
		return os.Stderr
	}
	return os.Stdout
}
