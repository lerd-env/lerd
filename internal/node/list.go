package node

// ListInstalled returns every Node major version installed under the active
// version manager, in the order it reports them. Empty when the manager isn't
// available or the user has no versions installed. Centralises listing so every
// surface (web UI, MCP, TUI) sees the same list with the same dedupe rules.
func ListInstalled() []string {
	return Active().List()
}
