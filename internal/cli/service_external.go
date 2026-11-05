package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

func newServiceExternalCmd() *cobra.Command {
	var req serviceops.ExternalRequest
	var passwordStdin, update bool
	cmd := &cobra.Command{
		Use:   "external <name>",
		Short: "Add a MySQL, MariaDB or PostgreSQL server lerd does not run, such as DBngin, as a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			req.Name = args[0]
			password, err := externalPassword(passwordStdin)
			if err != nil {
				return err
			}
			req.Password = password
			if update {
				return UpdateExternalDBPassword(req.Name, password)
			}
			svc, err := serviceops.AddExternalDB(req)
			if err != nil {
				return err
			}
			feedback.Done(fmt.Sprintf("Added %s, using the server at %s:%d", svc.Name, svc.External.Host, svc.External.Port))
			return nil
		},
	}
	cmd.Flags().StringVar(&req.Engine, "engine", "mysql", "mysql, mariadb or postgres")
	cmd.Flags().StringVar(&req.Host, "host", "127.0.0.1", "Where the server listens; 127.0.0.1 means this machine")
	cmd.Flags().IntVar(&req.Port, "port", 3306, "The server's port")
	cmd.Flags().StringVar(&req.User, "user", "root", "A user allowed to list, create and drop databases")
	cmd.Flags().BoolVar(&update, "update", false, "Change the password of a server already added, and of the sites using it")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin rather than asking for it")
	return cmd
}

// externalPassword never comes from a flag, where any process could read it.
func externalPassword(fromStdin bool) (string, error) {
	if !fromStdin {
		return readPasswordOnce()
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// UpdateExternalDBPassword changes a server's password, then rewrites the env of
// every site on it through lerd env, whose run carries it to their worktrees.
func UpdateExternalDBPassword(name, password string) error {
	if err := serviceops.UpdateExternalDBPassword(name, password); err != nil {
		return err
	}
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	for _, site := range reg.Sites {
		if svc := config.ExternalDBServiceFor(site.Path); svc == nil || svc.Name != name {
			continue
		}
		cmd := exec.Command(self, "env")
		cmd.Dir = site.Path
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("updating %s: %s", site.Name, strings.TrimSpace(string(out)))
		}
		feedback.Note("updated " + site.Name)
	}
	feedback.Done("Changed the password of " + name)
	return nil
}
