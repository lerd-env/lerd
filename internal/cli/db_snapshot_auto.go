package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

// NewDbSnapshotKeepCmd returns the standalone db:snapshot:keep command.
func NewDbSnapshotKeepCmd() *cobra.Command { return newDbSnapshotKeepCmd("db:snapshot:keep") }

// NewDbSnapshotAutoCmd returns the standalone db:snapshot:auto command.
func NewDbSnapshotAutoCmd() *cobra.Command { return newDbSnapshotAutoCmd("db:snapshot:auto") }

func newDbSnapshotKeepCmd(use string) *cobra.Command {
	var service, database string
	var allDatabases, release bool
	cmd := &cobra.Command{
		Use:   use + " <name>",
		Short: "Keep an automatic snapshot for good, exempt from retention",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runDbSnapshotKeep(args[0], service, database, allDatabases, !release)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Lerd DB service to target (e.g. mysql, postgres)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "Database name (default: from .env or .lerd.yaml)")
	cmd.Flags().BoolVarP(&allDatabases, "all-databases", "A", false, "Target an all-databases snapshot")
	cmd.Flags().BoolVar(&release, "off", false, "Put the snapshot back under retention")
	return cmd
}

func newDbSnapshotAutoCmd(use string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: "Enable, disable, or show scheduled database snapshots",
		Long: `Configure scheduled database snapshots: every site the policy covers has its
database snapshotted on a schedule, through the same machinery db:snapshot uses.
Retention only ever prunes automatic snapshots, and never one you kept with
db:snapshot:keep. The schedule ships on in opt-in mode, so it covers nothing
until a site opts in with db:snapshot:auto site <name> on.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return runDbSnapshotAutoStatus() },
	}
	cmd.AddCommand(newDbSnapshotAutoOnCmd(), newDbSnapshotAutoOffCmd(), newDbSnapshotAutoSiteCmd())
	cmd.AddCommand(&cobra.Command{
		Use: "status", Short: "Show the automatic-snapshot policy and what it covers", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return runDbSnapshotAutoStatus() },
	})
	return cmd
}

func newDbSnapshotAutoOnCmd() *cobra.Command {
	var every, keepFor, selection string
	var keep int
	cmd := &cobra.Command{
		Use:   "on",
		Short: "Enable scheduled snapshots",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			opts := autoSnapshotOptions{}
			if c.Flags().Changed("every") {
				opts.every = &every
			}
			if c.Flags().Changed("keep") {
				opts.keep = &keep
			}
			if c.Flags().Changed("keep-for") {
				opts.keepFor = &keepFor
			}
			if c.Flags().Changed("selection") {
				opts.selection = &selection
			}
			return runDbSnapshotAutoToggle(true, opts)
		},
	}
	cmd.Flags().StringVar(&every, "every", "", "How often to snapshot, as a duration (e.g. 6h, 24h)")
	cmd.Flags().IntVar(&keep, "keep", 0, "How many automatic snapshots to keep per database (-1 for no limit)")
	cmd.Flags().StringVar(&keepFor, "keep-for", "", "Also drop automatic snapshots older than this (e.g. 168h)")
	cmd.Flags().StringVar(&selection, "selection", "", "opt-out covers every site until one is excluded; opt-in covers none until one is included")
	return cmd
}

func newDbSnapshotAutoOffCmd() *cobra.Command {
	return &cobra.Command{
		Use: "off", Short: "Disable scheduled snapshots", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return runDbSnapshotAutoToggle(false, autoSnapshotOptions{}) },
	}
}

func newDbSnapshotAutoSiteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "site [name] <on|off|default>",
		Short: "Opt one site in or out of the schedule, or have it follow the global policy",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			name, mode := "", args[0]
			if len(args) == 2 {
				name, mode = args[0], args[1]
			}
			return runDbSnapshotAutoSite(name, mode)
		},
	}
}

// autoSnapshotOptions carries only the settings the user actually passed, so
// enabling the schedule never silently rewrites a retention they already chose.
type autoSnapshotOptions struct {
	every     *string
	keep      *int
	keepFor   *string
	selection *string
}

func runDbSnapshotKeep(name, service, database string, allDatabases, kept bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env, err := resolveDBLenient(cwd, service, database)
	if err != nil {
		return err
	}
	if err := serviceops.SetSnapshotKept(env.service, env.database, name, allDatabases, kept); err != nil {
		return err
	}
	feedback.Begin()
	if kept {
		feedback.Done("snapshot " + name + " will be kept, retention leaves it alone")
		return nil
	}
	feedback.Done("snapshot " + name + " is back under retention")
	return nil
}

func runDbSnapshotAutoToggle(enable bool, opts autoSnapshotOptions) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if opts.every != nil {
		if _, err := time.ParseDuration(*opts.every); err != nil {
			return fmt.Errorf("--every %q is not a duration, try 6h or 24h", *opts.every)
		}
		cfg.AutoSnapshot.Every = *opts.every
	}
	if opts.keep != nil {
		cfg.AutoSnapshot.Keep = *opts.keep
	}
	if opts.keepFor != nil {
		if *opts.keepFor != "" {
			if _, err := time.ParseDuration(*opts.keepFor); err != nil {
				return fmt.Errorf("--keep-for %q is not a duration, try 168h", *opts.keepFor)
			}
		}
		cfg.AutoSnapshot.KeepFor = *opts.keepFor
	}
	if opts.selection != nil {
		clean, err := config.NormalizeAutoSnapshotSelection(*opts.selection)
		if err != nil {
			return err
		}
		cfg.AutoSnapshot.Selection = clean
	}
	cfg.AutoSnapshot.Enabled = enable
	if err := config.SaveGlobal(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	feedback.Begin()
	if !enable {
		feedback.Done("Automatic snapshots disabled.")
		return nil
	}
	feedback.Done("Automatic snapshots enabled: " + autoSnapshotScheduleLine(cfg))
	return nil
}

func runDbSnapshotAutoSite(name, mode string) error {
	clean, err := config.NormalizeAutoSnapshotMode(mode)
	if err != nil {
		return err
	}
	if name == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		site, err := config.FindSiteByPath(cwd)
		if err != nil {
			return fmt.Errorf("no linked site here — pass a site name")
		}
		name = site.Name
	}
	if err := config.SetSiteAutoSnapshot(name, clean); err != nil {
		return err
	}
	feedback.Begin()
	switch clean {
	case config.AutoSnapshotOn:
		feedback.Done(name + " is always snapshotted on the schedule")
	case config.AutoSnapshotOff:
		feedback.Done(name + " is left out of the schedule")
	default:
		feedback.Done(name + " follows the global automatic-snapshot policy")
	}
	return nil
}

func runDbSnapshotAutoStatus() error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	state := feedback.Amber("disabled")
	if cfg.AutoSnapshotEnabled() {
		state = feedback.Green("enabled")
	}
	fmt.Printf("Automatic snapshots: %s\n", state)
	fmt.Printf("Schedule: %s\n", autoSnapshotScheduleLine(cfg))

	targets := config.AutoSnapshotTargets(cfg)
	if len(targets) == 0 {
		feedback.Line("no site is covered right now")
		return nil
	}
	every := cfg.AutoSnapshotEvery()
	rows := make([][]string, 0, len(targets))
	for _, t := range targets {
		last := lastAutoSnapshot(t)
		lastCell, nextCell := "never", "at the next check"
		if !last.IsZero() {
			lastCell = compactDuration(time.Since(last)) + " ago"
			nextCell = "in " + compactDuration(time.Until(last.Add(every)))
			if due := last.Add(every); !due.After(time.Now()) {
				nextCell = "at the next check"
			}
		}
		rows = append(rows, []string{t.Site, t.Database, t.Service, lastCell, nextCell})
	}
	feedback.Table([]string{"SITE", "DATABASE", "ENGINE", "LAST", "NEXT"}, rows)
	return nil
}

// autoSnapshotScheduleLine renders the effective policy in one sentence, the
// same wording the enable confirmation and the status header use.
func autoSnapshotScheduleLine(cfg *config.GlobalConfig) string {
	line := "every " + humanEvery(cfg.AutoSnapshotEvery())
	if keep := cfg.AutoSnapshotKeep(); keep > 0 {
		line += ", keeping " + strconv.Itoa(keep) + " per database"
	} else {
		line += ", no count limit"
	}
	if keepFor := cfg.AutoSnapshotKeepFor(); keepFor > 0 {
		line += ", dropping any older than " + humanEvery(keepFor)
	}
	if cfg.AutoSnapshotSelection() == config.AutoSnapshotOptIn {
		return line + ", covering only the sites that opted in"
	}
	return line + ", covering every site that has not opted out"
}

// humanEvery renders a schedule duration without the empty trailing units Go's
// own formatting leaves behind, so a daily schedule reads as 24h.
func humanEvery(d time.Duration) string {
	out := d.String()
	if strings.HasSuffix(out, "m0s") {
		out = strings.TrimSuffix(out, "0s")
	}
	if strings.HasSuffix(out, "h0m") {
		out = strings.TrimSuffix(out, "0m")
	}
	return out
}

// lastAutoSnapshot returns when the schedule last snapshotted a target, read off
// the snapshots themselves so the answer stays true even if the watcher's
// throttle file is lost.
func lastAutoSnapshot(t config.AutoSnapshotTarget) time.Time {
	snaps, err := serviceops.ListSnapshots(t.Service, t.Database, false)
	if err != nil {
		return time.Time{}
	}
	for _, s := range snaps { // newest first
		if s.Auto {
			return s.Created
		}
	}
	return time.Time{}
}
