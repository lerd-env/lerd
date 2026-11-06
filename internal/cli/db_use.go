package cli

import (
	"fmt"
	"os"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewDbUseCmd returns the db:use command.
func NewDbUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db:use <service>",
		Short: "Point this project at a database service, such as an external server you added",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if !config.IsDBServiceName(args[0]) || !config.CustomServiceExists(args[0]) {
				return fmt.Errorf("%s is not a database service you added; see lerd service list", args[0])
			}
			if err := config.ReplaceProjectDBService(cwd, args[0]); err != nil {
				return err
			}
			if err := NewEnvCmd().RunE(nil, nil); err != nil {
				return err
			}
			feedback.Done("This project now uses " + args[0])
			return nil
		},
	}
}
