package cli

import (
	"fmt"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/spf13/cobra"
)

// NewSitesRestoreCmd returns the sites:restore command.
func NewSitesRestoreCmd() *cobra.Command {
	var list bool
	cmd := &cobra.Command{
		Use:   "sites:restore [backup]",
		Short: "Put the site registry back from one of its automatic backups",
		Long: `Lerd copies its site registry aside before every change that rewrites it, so
a registry that loses sites can be put back without relinking them one by one.

With no argument the newest backup is restored. The registry it replaces is
backed up in turn, so restoring the wrong one is undone by restoring again.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runSitesRestore(name, list)
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "Show the available backups without restoring anything")
	return cmd
}

func runSitesRestore(name string, list bool) error {
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
