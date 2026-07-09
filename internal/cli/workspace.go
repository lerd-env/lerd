package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/spf13/cobra"
)

// NewWorkspaceCmd returns the workspace command. Workspaces group sites for
// display only, in the dashboard and the TUI. They are unrelated to `lerd
// group`, which binds a main site's subdomains and rewrites vhosts and certs.
func NewWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Group sites into workspaces for the dashboard and the TUI",
		Long: "Workspaces are a personal, display-only way to organise sites. " +
			"They never change how a site is served; for subdomain grouping see `lerd group`.",
	}
	cmd.AddCommand(newWorkspaceAddCmd())
	cmd.AddCommand(newWorkspaceRenameCmd())
	cmd.AddCommand(newWorkspaceRemoveCmd())
	cmd.AddCommand(newWorkspaceAssignCmd())
	cmd.AddCommand(newWorkspaceMoveCmd())
	cmd.AddCommand(newWorkspaceListCmd())
	return cmd
}

func newWorkspaceAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create an empty workspace",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceAdd,
	}
}

func newWorkspaceRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a workspace, keeping its sites",
		Args:  cobra.ExactArgs(2),
		RunE:  runWorkspaceRename,
	}
}

func newWorkspaceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Delete a workspace; its sites become ungrouped",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkspaceRemove,
	}
}

func newWorkspaceAssignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "assign <site> <workspace|none>",
		Short: "Move a site into a workspace, or out of one with 'none'",
		Args:  cobra.ExactArgs(2),
		RunE:  runWorkspaceAssign,
	}
}

func newWorkspaceMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <name> <position>",
		Short: "Reposition a workspace in the display order (0 is first)",
		Args:  cobra.ExactArgs(2),
		RunE:  runWorkspaceMove,
	}
}

func newWorkspaceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the workspaces and their sites",
		Args:  cobra.NoArgs,
		RunE:  runWorkspaceList,
	}
}

// notifyWorkspaceChange nudges a running dashboard to re-read the config.
// Best-effort: with no daemon running there is nothing to refresh.
func notifyWorkspaceChange() {
	_, _, _ = postUnix("/api/internal/notify", nil)
}

func runWorkspaceAdd(_ *cobra.Command, args []string) error {
	if err := config.AddWorkspace(args[0]); err != nil {
		return err
	}
	notifyWorkspaceChange()
	feedback.Begin()
	feedback.Done("created workspace " + feedback.Val(strings.TrimSpace(args[0])))
	return nil
}

func runWorkspaceRename(_ *cobra.Command, args []string) error {
	if err := config.RenameWorkspace(args[0], args[1]); err != nil {
		return err
	}
	notifyWorkspaceChange()
	feedback.Begin()
	feedback.Done("renamed workspace to " + feedback.Val(strings.TrimSpace(args[1])))
	return nil
}

func runWorkspaceRemove(_ *cobra.Command, args []string) error {
	if err := config.DeleteWorkspace(args[0]); err != nil {
		return err
	}
	notifyWorkspaceChange()
	feedback.Begin()
	feedback.Done("deleted workspace " + feedback.Val(args[0]))
	feedback.Note("its sites are now ungrouped")
	return nil
}

func runWorkspaceAssign(_ *cobra.Command, args []string) error {
	site, err := resolveWorkspaceSite(args[0])
	if err != nil {
		return err
	}
	workspace := strings.TrimSpace(args[1])
	if strings.EqualFold(workspace, "none") {
		workspace = ""
	}
	if err := config.AssignSiteWorkspace([]string{site.Name}, workspace, false); err != nil {
		return err
	}
	notifyWorkspaceChange()
	feedback.Begin()
	if workspace == "" {
		feedback.Done("ungrouped " + site.Name)
	} else {
		feedback.Done("moved " + site.Name + " into " + feedback.Val(workspace))
	}
	return nil
}

func runWorkspaceMove(_ *cobra.Command, args []string) error {
	pos, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("position must be a number, got %q", args[1])
	}
	if err := config.MoveWorkspace(args[0], pos); err != nil {
		return err
	}
	notifyWorkspaceChange()
	feedback.Begin()
	feedback.Done("moved workspace " + feedback.Val(args[0]))
	return nil
}

func runWorkspaceList(_ *cobra.Command, _ []string) error {
	workspaces, err := config.ListWorkspaces()
	if err != nil {
		return err
	}
	reg, err := config.LoadSites()
	if err != nil {
		return err
	}

	grouped := map[string]bool{}
	for _, ws := range workspaces {
		for _, name := range ws.Sites {
			grouped[name] = true
		}
	}
	var ungrouped []string
	for _, s := range reg.Sites {
		if !grouped[s.Name] {
			ungrouped = append(ungrouped, s.Name)
		}
	}

	if len(workspaces) == 0 {
		fmt.Println("No workspaces.")
	}
	for _, ws := range workspaces {
		fmt.Println(ws.Name)
		for _, name := range ws.Sites {
			fmt.Printf("  %s\n", name)
		}
	}
	if len(ungrouped) > 0 {
		fmt.Println("Ungrouped")
		for _, name := range ungrouped {
			fmt.Printf("  %s\n", name)
		}
	}
	return nil
}

// resolveWorkspaceSite accepts a site name or any of its domains.
func resolveWorkspaceSite(arg string) (*config.Site, error) {
	site, err := config.FindSite(arg)
	if err == nil {
		return site, nil
	}
	if site, err = config.FindSiteByDomain(arg); err == nil {
		return site, nil
	}
	return nil, fmt.Errorf("site %q not found", arg)
}
