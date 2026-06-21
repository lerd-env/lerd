package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

// NewDbMoveCmd returns the standalone db:move command.
func NewDbMoveCmd() *cobra.Command { return newDbMoveCmd("db:move") }

func newDbMoveCmd(use string) *cobra.Command {
	var from, to string
	var sites []string
	var all, force bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "Move site databases from one service to another in the same family",
		Long: `Move one or more sites' databases between two installed services in the same
family (e.g. postgres -> postgres-18, or mysql-5-7 -> mysql).

For each site it dumps the database from the source service, creates and restores
it on the target, then rewrites that site's .env (DB_HOST/DB_PORT) to point at the
target. The source data is left intact as a safety net.

Run without flags for an interactive wizard, or script it with --from/--to and
either --all or one or more --site flags.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDbMove(from, to, sites, all, force)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source service (e.g. postgres)")
	cmd.Flags().StringVar(&to, "to", "", "Target service (e.g. postgres-18)")
	cmd.Flags().StringArrayVar(&sites, "site", nil, "Site to move by name (repeatable)")
	cmd.Flags().BoolVar(&all, "all", false, "Move every site currently on the source service")
	cmd.Flags().BoolVar(&force, "force", false, "Skip the confirmation prompt")
	return cmd
}

// supportedMoveFamily reports whether a service family supports the dump/restore
// move. Only the SQL families whose dumps replay cleanly across versions qualify.
func supportedMoveFamily(family string) bool {
	switch family {
	case "mysql", "mariadb", "postgres":
		return true
	}
	return false
}

// validateMovePair checks that a from/to service pair is a legal move: distinct
// services in the same supported family.
func validateMovePair(from, to string) error {
	if from == "" || to == "" {
		return fmt.Errorf("both a source and target service are required")
	}
	if from == to {
		return fmt.Errorf("source and target are the same service (%q)", from)
	}
	ff := config.FamilyOfName(from)
	tf := config.FamilyOfName(to)
	if !supportedMoveFamily(ff) {
		return fmt.Errorf("source %q is not a movable database service (supported families: mysql, mariadb, postgres)", from)
	}
	if ff != tf {
		return fmt.Errorf("cannot move across families: %q is %s but %q is %s", from, ff, to, familyLabel(tf))
	}
	return nil
}

func familyLabel(f string) string {
	if f == "" {
		return "not a database service"
	}
	return f
}

// siteDBService returns the concrete DB service a site currently uses (e.g.
// "postgres-18"), reading the explicit choice from .lerd.yaml first and falling
// back to the container hostname in .env. Returns "" for sqlite or when no lerd
// DB service can be determined.
func siteDBService(sitePath string) string {
	if pc, err := config.LoadProjectConfig(sitePath); err == nil {
		if pc.DB.Service != "" && pc.DB.Service != "sqlite" {
			return pc.DB.Service
		}
		for _, svc := range pc.Services {
			if svc.Name != "sqlite" && config.IsDBServiceName(svc.Name) {
				return svc.Name
			}
		}
	}
	host := strings.TrimSpace(envfile.ReadKey(filepath.Join(sitePath, ".env"), "DB_HOST"))
	if strings.HasPrefix(host, "lerd-") {
		return strings.TrimPrefix(host, "lerd-")
	}
	return ""
}

func findSiteByName(reg *config.SiteRegistry, name string) *config.Site {
	for i := range reg.Sites {
		if reg.Sites[i].Name == name {
			return &reg.Sites[i]
		}
	}
	return nil
}

// sitesOnService returns every registered site whose current DB service is svc.
func sitesOnService(reg *config.SiteRegistry, svc string) []config.Site {
	var out []config.Site
	for _, s := range reg.Sites {
		if siteDBService(s.Path) == svc {
			out = append(out, s)
		}
	}
	return out
}

// resolveMoveSites turns the --all / --site selection into a concrete site list.
// With --all it returns every site detected on the source service; with named
// sites it validates each exists and is not already on a different service.
func resolveMoveSites(reg *config.SiteRegistry, from string, names []string, all bool) ([]config.Site, error) {
	if all {
		return sitesOnService(reg, from), nil
	}
	var out []config.Site
	for _, name := range names {
		s := findSiteByName(reg, name)
		if s == nil {
			return nil, fmt.Errorf("unknown site %q", name)
		}
		if detected := siteDBService(s.Path); detected != "" && detected != from {
			return nil, fmt.Errorf("site %q is on %q, not %q", name, detected, from)
		}
		out = append(out, *s)
	}
	return out, nil
}

func runDbMove(from, to string, siteNames []string, all, force bool) error {
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}

	interactive := isInteractive()
	if from == "" || to == "" || (!all && len(siteNames) == 0) {
		if !interactive {
			return fmt.Errorf("non-interactive: provide --from, --to, and either --all or --site <name>")
		}
		from, to, siteNames, err = dbMoveWizard(reg, from, to)
		if err != nil {
			return err
		}
		all = false
	}

	if err := validateMovePair(from, to); err != nil {
		return err
	}
	if !serviceops.ServiceInstalled(to) {
		return fmt.Errorf("target service %q is not installed — add it from the Services tab or with `lerd service preset %s`", to, config.FamilyOfName(to))
	}

	targets, err := resolveMoveSites(reg, from, siteNames, all)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no sites to move from %q", from)
	}

	if !force && interactive {
		feedback.Begin()
		feedback.Line(fmt.Sprintf("about to move %d site(s) from %s to %s:", len(targets), from, to))
		for _, s := range targets {
			feedback.Note(s.Name + " (" + s.Path + ")")
		}
		if !feedback.Confirm("Source data is left intact. Continue?", false) {
			return fmt.Errorf("aborted")
		}
	}

	failed := 0
	for _, s := range targets {
		feedback.Line(fmt.Sprintf("moving %s: %s → %s", s.Name, from, to))
		if err := runDbMoveOne(s.Path, s.Name, from, to); err != nil {
			// Per-site failure detail goes to stderr so scripts that parse the
			// error stream still see which sites failed (the command also
			// returns a non-nil aggregate error below).
			fmt.Fprintf(os.Stderr, "  %s %s: %v\n", feedback.Red(feedback.GlyphFail), s.Name, err)
			failed++
			continue
		}
		feedback.Note(s.Name + " moved · .env updated")
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d site(s) failed to move", failed, len(targets))
	}
	feedback.Done(fmt.Sprintf("moved %d site(s) %s → %s · source data left intact", len(targets), from, to))
	return nil
}

// runDbMoveOne dumps a single site's database from the source service, repoints
// the site's config + .env at the target (which creates the empty database), and
// restores the dump into the target.
func runDbMoveOne(sitePath, siteName, from, to string) error {
	base, err := resolveDB(sitePath, "", "")
	if err != nil {
		return fmt.Errorf("cannot resolve database from .env: %w", err)
	}
	if base.database == "" {
		return fmt.Errorf("no database name found in .env")
	}

	if err := ensureServiceRunning(from); err != nil {
		return fmt.Errorf("could not start %s: %w", from, err)
	}

	tmp, err := os.CreateTemp("", "lerd-dbmove-*.sql")
	if err != nil {
		return fmt.Errorf("creating temp dump: %w", err)
	}
	defer os.Remove(tmp.Name())

	srcEnv := serviceToDBEnv(from)
	srcEnv.database = base.database
	dump, err := dbExportCmd(srcEnv)
	if err != nil {
		tmp.Close() //nolint:errcheck
		return err
	}
	dump.Stdout = tmp
	dump.Stderr = os.Stderr
	feedback.Note("dumping " + base.database + " from " + from)
	if err := dump.Run(); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("dump failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing dump: %w", err)
	}

	// Repoint the site at the target service. This mirrors db_set: it rewrites
	// the DB_ keys in .env (host-proxy aware), starts the target, and creates
	// the database, so the dump has somewhere to land. Snapshot .lerd.yaml's
	// source and the .env bytes first so a failed repoint can be fully undone.
	feedback.Note("repointing " + siteName + " at " + to)
	envPath := filepath.Join(sitePath, ".env")
	prevEnv, _ := os.ReadFile(envPath)
	if err := config.ReplaceProjectDBService(sitePath, to); err != nil {
		return fmt.Errorf("saving .lerd.yaml: %w", err)
	}
	// Once the site is repointed at the target, any later failure has to undo
	// .lerd.yaml and .env so the live site keeps pointing at the source (whose
	// data is still intact) rather than an empty or half-restored target DB.
	rollback := func(cause error) error {
		rbErr := config.ReplaceProjectDBService(sitePath, from)
		if prevEnv != nil {
			if wErr := os.WriteFile(envPath, prevEnv, 0o644); wErr != nil && rbErr == nil {
				rbErr = wErr
			}
		}
		if rbErr != nil {
			return fmt.Errorf("%w (rollback also failed: %v)", cause, rbErr)
		}
		return cause
	}
	if err := runLerdEnv(sitePath); err != nil {
		return rollback(fmt.Errorf("applying env for target: %w", err))
	}

	// The target database name is whatever env setup landed on. env starts the
	// target and creates the DB on a best-effort basis, so make both certain
	// before restoring rather than letting a warned-over failure surface here.
	tgt, err := resolveDB(sitePath, "", "")
	if err != nil {
		return rollback(fmt.Errorf("cannot resolve target database: %w", err))
	}
	if err := ensureServiceRunning(to); err != nil {
		return rollback(fmt.Errorf("could not start target %s: %w", to, err))
	}
	if _, err := createDatabase(to, tgt.database); err != nil {
		return rollback(fmt.Errorf("creating database %q on %s: %w", tgt.database, to, err))
	}
	tgtEnv := serviceToDBEnv(to)
	tgtEnv.database = tgt.database

	f, err := os.Open(tmp.Name())
	if err != nil {
		return rollback(fmt.Errorf("reopening dump: %w", err))
	}
	defer f.Close()
	restore, err := dbImportCmd(tgtEnv)
	if err != nil {
		return rollback(err)
	}
	restore.Stdin = f
	restore.Stdout = os.Stdout
	restore.Stderr = os.Stderr
	feedback.Note("restoring " + tgtEnv.database + " into " + to)
	if err := restore.Run(); err != nil {
		return rollback(fmt.Errorf("restore failed: %w", err))
	}
	return nil
}

// runLerdEnv re-execs `lerd env` in the project directory so the existing env
// setup (service start, .env rewrite, database provisioning) runs unchanged.
func runLerdEnv(dir string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving lerd executable: %w", err)
	}
	cmd := exec.Command(self, "env")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dbMoveWizard interactively collects the source service, target service, and
// sites to move. Pre-supplied from/to skip the corresponding prompt.
func dbMoveWizard(reg *config.SiteRegistry, from, to string) (string, string, []string, error) {
	// Count sites per movable source service.
	siteCounts := map[string]int{}
	for _, s := range reg.Sites {
		if svc := siteDBService(s.Path); svc != "" && supportedMoveFamily(config.FamilyOfName(svc)) {
			siteCounts[svc]++
		}
	}

	if from == "" {
		if len(siteCounts) == 0 {
			return "", "", nil, fmt.Errorf("no sites are using a movable database service")
		}
		opts := make([]huh.Option[string], 0, len(siteCounts))
		for _, svc := range sortedCountKeys(siteCounts) {
			label := fmt.Sprintf("%s (%d site%s)", svc, siteCounts[svc], pluralS(siteCounts[svc]))
			opts = append(opts, huh.NewOption(label, svc))
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Move databases from which service?").
				Options(opts...).
				Value(&from),
		)).WithTheme(huh.ThemeCatppuccin()).Run(); err != nil {
			return "", "", nil, err
		}
	}

	family := config.FamilyOfName(from)
	if to == "" {
		var targets []string
		for _, host := range config.ServicesInFamily(family) {
			name := strings.TrimPrefix(host, "lerd-")
			if name != from && serviceops.ServiceInstalled(name) {
				targets = append(targets, name)
			}
		}
		if len(targets) == 0 {
			return "", "", nil, fmt.Errorf("no other installed %s service to move to — install an alternate first (Services tab or `lerd service preset %s`)", family, family)
		}
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title("Move to which service?").
				Options(huh.NewOptions(targets...)...).
				Value(&to),
		)).WithTheme(huh.ThemeCatppuccin()).Run(); err != nil {
			return "", "", nil, err
		}
	}

	var siteOpts []string
	for _, s := range sitesOnService(reg, from) {
		siteOpts = append(siteOpts, s.Name)
	}
	if len(siteOpts) == 0 {
		return "", "", nil, fmt.Errorf("no sites found on %q", from)
	}
	selected := append([]string(nil), siteOpts...)
	if err := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title(fmt.Sprintf("Which sites to move from %s to %s?", from, to)).
			Description("All selected by default; deselect any you want to leave behind.").
			Options(huh.NewOptions(siteOpts...)...).
			Value(&selected),
	)).WithTheme(huh.ThemeCatppuccin()).Run(); err != nil {
		return "", "", nil, err
	}
	if len(selected) == 0 {
		return "", "", nil, fmt.Errorf("no sites selected")
	}
	return from, to, selected, nil
}

func sortedCountKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
