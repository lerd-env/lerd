package cli

import (
	"fmt"
	"slices"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/spf13/cobra"
)

// NewSitesRestoreCmd returns the sites:restore command.
func NewSitesRestoreCmd() *cobra.Command {
	var list, force bool
	cmd := &cobra.Command{
		Use:   "sites:restore [backup]",
		Short: "Put the site registry back from one of its automatic backups",
		Long: `Lerd copies its site registry aside before every change that rewrites it, so
a registry that loses sites can be put back without relinking them one by one.

With no argument the newest backup is restored. What the restore would change is
shown first and confirmed, because a backup carries whatever was true when it was
taken: a PHP pin, a domain or a TLS state moved back can stop a site serving. The
registry it replaces is backed up in turn, so restoring the wrong one is undone by
restoring again.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runSitesRestore(name, list, force)
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "Show the available backups without restoring anything")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip the confirmation prompt")
	return cmd
}

func runSitesRestore(name string, list, force bool) error {
	backups, err := config.ListSitesBackups()
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("no site registry backup available yet")
	}

	if list {
		fmt.Println()
		for _, b := range backups {
			fmt.Printf("  %s  %s  %s\n",
				b.Name,
				b.Time.Format("2006-01-02 15:04:05"),
				feedback.Dim(fmt.Sprintf("%d site(s)", b.Sites)))
		}
		fmt.Println()
		fmt.Println("Restore one with: lerd sites:restore <name>")
		return nil
	}

	if !force {
		if err := confirmSitesRestore(name, backups); err != nil {
			return err
		}
	}

	reg, used, err := config.RestoreSitesBackup(name)
	if err != nil {
		return err
	}

	feedback.Begin()
	feedback.Start("restoring the site registry").OK(feedback.Val(used))
	for _, site := range reg.Sites {
		if err := nginx.RegenerateVhost(site); err != nil {
			feedback.Warn("regenerating vhost for %s: %v", site.Name, err)
		}
	}
	nginx.ReloadOrWarn("  ")
	feedback.Done(fmt.Sprintf("%d site(s) restored", len(reg.Sites)))
	fmt.Println("Run 'lerd start' to bring their containers and workers back up.")
	return nil
}

// confirmSitesRestore shows what the restore would change and asks before it
// happens. Without this the command rewrote live state from a file the user
// never saw: the newest backup carries whatever was true when it was taken, so
// a restore can silently move a working site back onto a broken PHP pin.
func confirmSitesRestore(name string, backups []config.SitesBackup) error {
	target := name
	if target == "" {
		// RestoreSitesBackup takes the newest for an empty name, and
		// ListSitesBackups is sorted newest first, so this is the same file.
		target = backups[0].Name
	} else if !slices.ContainsFunc(backups, func(b config.SitesBackup) bool { return b.Name == target }) {
		// Same refusal RestoreSitesBackup would give, raised before the diff so a
		// bad name reads the same whether or not the prompt is skipped.
		return fmt.Errorf("no such backup: %s", target)
	}
	backup, err := config.ReadSitesBackup(target)
	if err != nil {
		return fmt.Errorf("reading backup %s: %w", target, err)
	}
	var currentSites []config.Site
	if cur, err := config.LoadSites(); err == nil {
		currentSites = cur.Sites
	}

	changes := diffSiteRegistries(currentSites, backup.Sites)
	fmt.Println()
	fmt.Printf("  Restoring %s (%d site(s))\n", target, len(backup.Sites))
	if len(changes) == 0 {
		fmt.Println("  It matches the current registry, nothing would change.")
		return nil
	}
	fmt.Println()
	for _, c := range changes {
		fmt.Println("    " + c)
	}
	fmt.Println()

	if !isInteractive() {
		return fmt.Errorf("restoring %s rewrites the registry above — rerun with --force to confirm", target)
	}
	if !feedback.Confirm("Restore this registry?", false) {
		return fmt.Errorf("restore cancelled")
	}
	return nil
}
