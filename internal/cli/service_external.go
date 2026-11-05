package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

func newServiceExternalCmd() *cobra.Command {
	var req serviceops.ExternalRequest
	var passwordStdin bool
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
