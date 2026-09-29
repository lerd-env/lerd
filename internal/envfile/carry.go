package envfile

// CarriedValues is what a worktree env takes from its parent's change from
// before to after: each value the parent moved, where the worktree still holds
// the parent's old one. A value the worktree has made its own, like its own
// database or URL, never equalled the parent's and stays put.
func CarriedValues(worktree, before, after map[string]string) map[string]string {
	carried := map[string]string{}
	for k, old := range before {
		if v := after[k]; old != "" && v != "" && v != old && worktree[k] == old {
			carried[k] = v
		}
	}
	return carried
}
