package cli

import (
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// staleServiceCarry returns the parent's current values for every service block
// a worktree still names by container.
//
// A worktree created before the runtime switch kept the container names its
// parent has since had rewritten to loopback, so neither the host nor the port
// equals the parent's previous value any more and the carry that follows a
// parent's change walks past both. That is not a value the worktree made its
// own, it is one the parent already moved on from, so the whole block follows:
// taking the host without its port would point the worktree at loopback on the
// container's internal port, which is worse than leaving it alone.
//
// Scoped to the keys the framework declares for the service naming the
// container, so a worktree's own database and URL are never in reach.
func staleServiceCarry(fw *config.Framework, worktree, after map[string]string) map[string]string {
	carried := map[string]string{}
	if fw == nil {
		return carried
	}
	for name, def := range fw.Env.Services {
		keys := serviceVarKeys(def)
		if !namesContainer(worktree, keys, name) || namesContainer(after, keys, name) {
			continue
		}
		for _, k := range keys {
			if v := after[k]; v != "" && worktree[k] != v {
				carried[k] = v
			}
		}
	}
	return carried
}

// serviceVarKeys is the env keys a service writes that are the same for every
// checkout of a project. A var the store declares with a placeholder is named
// after the project, which is the database or the bucket, and a worktree is
// entitled to its own: carrying those would point an isolated worktree back at
// its parent's data.
func serviceVarKeys(def config.FrameworkServiceDef) []string {
	keys := make([]string, 0, len(def.Vars))
	for _, kv := range def.Vars {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" || strings.Contains(v, "{{") {
			continue
		}
		keys = append(keys, k)
	}
	return keys
}

// namesContainer reports whether any of a service's own keys still holds a
// reference to its container. Matched against this service's container rather
// than any "lerd-" text, so a database or bucket that merely happens to start
// with the prefix is not read as one.
func namesContainer(vals map[string]string, keys []string, service string) bool {
	needle := "lerd-" + service
	for _, k := range keys {
		if v := vals[k]; v != "" && strings.Contains(v, needle) {
			return true
		}
	}
	return false
}
