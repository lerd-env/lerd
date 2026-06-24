package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

// NewDbSnapshotCmd returns the standalone db:snapshot command.
func NewDbSnapshotCmd() *cobra.Command { return newDbSnapshotCmd("db:snapshot") }

// NewDbSnapshotsCmd returns the standalone db:snapshots command.
func NewDbSnapshotsCmd() *cobra.Command { return newDbSnapshotsCmd("db:snapshots") }

// NewDbRestoreCmd returns the standalone db:restore command.
func NewDbRestoreCmd() *cobra.Command { return newDbRestoreCmd("db:restore") }

// NewDbSnapshotRmCmd returns the standalone db:snapshot:rm command.
func NewDbSnapshotRmCmd() *cobra.Command { return newDbSnapshotRmCmd("db:snapshot:rm") }

func newDbSnapshotCmd(use string) *cobra.Command {
	var service, database string
	var allDatabases bool
	cmd := &cobra.Command{
		Use:   use + " [name]",
		Short: "Create a named snapshot of the project database",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			return runDbSnapshot(name, service, database, allDatabases)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Lerd DB service to target (e.g. mysql, postgres)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "Database name (default: from .env or .lerd.yaml)")
	cmd.Flags().BoolVarP(&allDatabases, "all-databases", "A", false, "Snapshot every database in the service")
	return cmd
}

func newDbSnapshotsCmd(use string) *cobra.Command {
	var service, database string
	var all bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "List stored database snapshots",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDbSnapshots(service, database, all)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Lerd DB service to target (e.g. mysql, postgres)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "Database name (default: from .env or .lerd.yaml)")
	cmd.Flags().BoolVar(&all, "all", false, "List snapshots across every database on the service")
	return cmd
}

func newDbRestoreCmd(use string) *cobra.Command {
	var service, database string
	var allDatabases, force bool
	cmd := &cobra.Command{
		Use:   use + " <name>",
		Short: "Restore a database from a stored snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDbRestore(args[0], service, database, allDatabases, force)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Lerd DB service to target (e.g. mysql, postgres)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "Database name (default: from .env or .lerd.yaml)")
	cmd.Flags().BoolVarP(&allDatabases, "all-databases", "A", false, "Restore an all-databases snapshot")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip the confirmation prompt")
	return cmd
}

func newDbSnapshotRmCmd(use string) *cobra.Command {
	var service, database string
	var allDatabases bool
	cmd := &cobra.Command{
		Use:   use + " <name>",
		Short: "Delete a stored database snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDbSnapshotRm(args[0], service, database, allDatabases)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Lerd DB service to target (e.g. mysql, postgres)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "Database name (default: from .env or .lerd.yaml)")
	cmd.Flags().BoolVarP(&allDatabases, "all-databases", "A", false, "Target an all-databases snapshot")
	return cmd
}

func runDbSnapshot(name, service, database string, allDatabases bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env, err := resolveDBForSnapshot(cwd, service, database, allDatabases)
	if err != nil {
		return err
	}
	target := snapshotTarget(env, allDatabases)
	if !serviceops.SnapshotFamilySupported(target.Family) {
		return fmt.Errorf("snapshots support only MySQL, MariaDB and PostgreSQL (service %q is %q)", env.service, target.Family)
	}
	if !target.AllDatabases && target.Database == "" {
		return fmt.Errorf("no database resolved — pass --database, or --all-databases to snapshot the whole service")
	}
	if err := ensureServiceRunning(env.service); err != nil {
		return fmt.Errorf("could not start %s: %w", env.service, err)
	}

	meta := serviceops.SnapshotMeta{Site: snapshotSiteName(cwd), GitBranch: snapshotGitBranch(cwd)}
	snap, err := serviceops.CreateSnapshot(target, name, meta, snapshotEmit())
	if err != nil {
		return err
	}
	scope := snap.Database
	if snap.AllDatabases {
		scope = "all databases"
	}
	feedback.Begin()
	feedback.Done(fmt.Sprintf("snapshot %s created for %s (%s)", snap.Name, scope, humanSize(snap.SizeBytes)))
	return nil
}

func runDbSnapshots(service, database string, all bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env, err := resolveDBLenient(cwd, service, database)
	if err != nil {
		return err
	}
	listDatabase := env.database
	if all {
		listDatabase = ""
	}
	snaps, err := serviceops.ListSnapshots(env.service, listDatabase, true)
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		feedback.Begin()
		if listDatabase == "" {
			feedback.Line("no snapshots for service " + env.service)
		} else {
			feedback.Line("no snapshots for " + listDatabase)
		}
		return nil
	}
	printSnapshotTable(snaps)
	return nil
}

func runDbRestore(name, service, database string, allDatabases, force bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env, err := resolveDBForSnapshot(cwd, service, database, allDatabases)
	if err != nil {
		return err
	}
	target := snapshotTarget(env, allDatabases)
	if !serviceops.SnapshotFamilySupported(target.Family) {
		return fmt.Errorf("snapshots support only MySQL, MariaDB and PostgreSQL (service %q is %q)", env.service, target.Family)
	}
	if !target.AllDatabases && target.Database == "" {
		return fmt.Errorf("no database resolved — pass --database, or --all-databases to restore the whole service")
	}

	scope := target.Database
	if allDatabases {
		scope = "all databases on " + env.service
	}
	if !force {
		if !isInteractive() {
			return fmt.Errorf("restoring %q overwrites %s — rerun with --force to confirm", name, scope)
		}
		if !feedback.Confirm(fmt.Sprintf("Restore snapshot %q into %s? This overwrites the current data.", name, scope), false) {
			return fmt.Errorf("restore cancelled")
		}
	}

	if err := ensureServiceRunning(env.service); err != nil {
		return fmt.Errorf("could not start %s: %w", env.service, err)
	}
	if err := serviceops.RestoreSnapshot(target, name, snapshotEmit()); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("snapshot " + name + " restored")
	return nil
}

func runDbSnapshotRm(name, service, database string, allDatabases bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env, err := resolveDBLenient(cwd, service, database)
	if err != nil {
		return err
	}
	if err := serviceops.DeleteSnapshot(env.service, env.database, name, allDatabases); err != nil {
		return err
	}
	feedback.Begin()
	feedback.Done("snapshot " + name + " deleted")
	return nil
}

// resolveDBForSnapshot resolves the project DB. All-databases operations only
// need the service, so they use lenient resolution that tolerates a missing
// database name.
func resolveDBForSnapshot(cwd, service, database string, allDatabases bool) (*dbEnv, error) {
	if allDatabases {
		return resolveDBLenient(cwd, service, database)
	}
	return resolveDB(cwd, service, database)
}

// snapshotTarget maps a resolved dbEnv onto a serviceops.SnapshotTarget.
func snapshotTarget(env *dbEnv, allDatabases bool) serviceops.SnapshotTarget {
	family := config.FamilyOfName(env.service)
	if family == "" {
		family = env.service
	}
	return serviceops.SnapshotTarget{
		Service:      env.service,
		Family:       family,
		Database:     env.database,
		AllDatabases: allDatabases,
	}
}

// snapshotEmit prints PhaseEvent progress messages during create and restore.
func snapshotEmit() func(serviceops.PhaseEvent) {
	return func(e serviceops.PhaseEvent) {
		if e.Message != "" {
			fmt.Printf("  %s\n", e.Message)
		}
	}
}

func snapshotSiteName(cwd string) string {
	if site, err := config.FindSiteByPath(cwd); err == nil {
		return site.Name
	}
	return ""
}

func snapshotGitBranch(cwd string) string {
	out, err := gitpkg.Output(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func printSnapshotTable(snaps []serviceops.Snapshot) {
	rows := make([][]string, 0, len(snaps))
	for _, s := range snaps {
		db := s.Database
		if s.AllDatabases {
			db = "(all)"
		}
		branch := s.GitBranch
		if branch == "" {
			branch = "-"
		}
		rows = append(rows, []string{
			s.Name, s.Created.Local().Format("2006-01-02 15:04"), db, humanSize(s.SizeBytes), branch,
		})
	}
	feedback.Table([]string{"NAME", "CREATED", "DATABASE", "SIZE", "BRANCH"}, rows)
}

// humanSize renders a byte count in binary units.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
