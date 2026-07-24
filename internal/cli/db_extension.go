package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/spf13/cobra"
)

// NewDbExtensionCmd returns the standalone db:extension command.
func NewDbExtensionCmd() *cobra.Command { return newDbExtensionCmd("db:extension") }

// newDbExtensionCmd manages the database extensions an engine's image can
// create. lerd creates one automatically when an imported dump reaches for it,
// so this is for the times you want one before any dump arrives.
func newDbExtensionCmd(use string) *cobra.Command {
	var database, service string
	cmd := &cobra.Command{
		Use:   use,
		Short: "List or add database extensions the engine can create",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runDbExtensionList(service, database) },
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "Show which extensions the engine offers and which the database has",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return runDbExtensionList(service, database) },
	}
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Create an extension in the database",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return runDbExtensionAdd(args[0], service, database) },
	}
	for _, c := range []*cobra.Command{cmd, list, add} {
		c.Flags().StringVarP(&database, "database", "d", "", "Database name (default: from .env or .lerd.yaml)")
		c.Flags().StringVarP(&service, "service", "s", "", "Lerd DB service to target (e.g. postgres)")
	}
	cmd.AddCommand(list, add)
	return cmd
}

// resolveExtensionTarget resolves the engine and database the same way every
// other db command does, and rejects engines that have no extensions at all.
func resolveExtensionTarget(service, database string) (*dbEnv, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	env, err := resolveDB(cwd, service, database)
	if err != nil {
		return nil, err
	}
	if env.connection != "pgsql" && env.connection != "postgres" {
		return nil, fmt.Errorf("%s databases have no extensions", env.connection)
	}
	if err := ensureServiceRunning(env.service); err != nil {
		return nil, fmt.Errorf("could not start %s: %w", env.service, err)
	}
	return env, nil
}

func runDbExtensionList(service, database string) error {
	env, err := resolveExtensionTarget(service, database)
	if err != nil {
		return err
	}
	installed, err := serviceops.InstalledExtensions(env.service, env.database)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, name := range installed {
		have[name] = true
	}
	declared := serviceops.DeclaredExtensions(env.service)
	fmt.Printf("Extensions in %s on %s:\n", env.database, env.service)
	for _, name := range installed {
		fmt.Printf("  %-20s installed\n", name)
	}
	for _, e := range declared {
		if have[e.Name] {
			continue
		}
		types := ""
		if len(e.Types) > 0 {
			types = "  (" + strings.Join(e.Types, ", ") + ")"
		}
		fmt.Printf("  %-20s available%s\n", e.Name, types)
	}
	if len(declared) == 0 {
		fmt.Println("  the engine declares none beyond what is already there")
	}
	return nil
}

func runDbExtensionAdd(name, service, database string) error {
	env, err := resolveExtensionTarget(service, database)
	if err != nil {
		return err
	}
	feedback.Begin()
	if err := serviceops.CreateExtension(env.service, env.database, name); err != nil {
		return err
	}
	feedback.Done("extension " + feedback.Val(name) + " ready in " + feedback.Val(env.database))
	return nil
}
