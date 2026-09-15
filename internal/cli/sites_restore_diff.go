package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// siteRestoreChange is one line of what a restore would do to the registry.
type siteRestoreChange struct {
	Site   string
	Detail string
}

// diffSiteRegistries reports what replacing current with backup would change.
// A restore rewrites live state, and the fields it silently moves are the ones
// that decide whether a site still serves: a PHP pin, a domain, the TLS state.
// Restoring a backup taken while a site was pinned to a version its
// dependencies cannot satisfy puts that site back on 500 with nothing said.
func diffSiteRegistries(current, backup []config.Site) []string {
	cur := indexSitesByName(current)
	bak := indexSitesByName(backup)

	var changes []siteRestoreChange
	for name, c := range cur {
		b, ok := bak[name]
		if !ok {
			changes = append(changes, siteRestoreChange{name, "removed from the registry"})
			continue
		}
		if fields := changedSiteFields(c, b); len(fields) > 0 {
			changes = append(changes, siteRestoreChange{name, strings.Join(fields, ", ")})
		}
	}
	for name := range bak {
		if _, ok := cur[name]; !ok {
			changes = append(changes, siteRestoreChange{name, "added back to the registry"})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Site < changes[j].Site })
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Site+": "+c.Detail)
	}
	return out
}

func indexSitesByName(sites []config.Site) map[string]config.Site {
	out := make(map[string]config.Site, len(sites))
	for _, s := range sites {
		out[s.Name] = s
	}
	return out
}

// changedSiteFields names the fields whose move a user would want to see before
// agreeing to it. Bookkeeping the daemon rewrites on its own is left out: it
// changes constantly and listing it would bury the fields that matter.
func changedSiteFields(cur, bak config.Site) []string {
	var out []string
	add := func(label, from, to string) {
		if from != to {
			out = append(out, fmt.Sprintf("%s %s → %s", label, orNone(from), orNone(to)))
		}
	}
	add("PHP", cur.PHPVersion, bak.PHPVersion)
	add("Node", cur.NodeVersion, bak.NodeVersion)
	add("framework", cur.Framework, bak.Framework)
	add("path", cur.Path, bak.Path)
	add("domains", strings.Join(cur.Domains, " "), strings.Join(bak.Domains, " "))
	add("TLS", strconv.FormatBool(cur.Secured), strconv.FormatBool(bak.Secured))
	// How the site is served, not just what it is. A backup taken mid-switch
	// carries the runtime that was live then, and restoring it repoints the
	// vhost at a per-site container that no longer exists: the site 502s with
	// nothing said, which is the failure this diff exists to prevent.
	add("runtime", runtimeName(cur.Runtime), runtimeName(bak.Runtime))
	add("runtime worker", strconv.FormatBool(cur.RuntimeWorker), strconv.FormatBool(bak.RuntimeWorker))
	add("public dir", cur.PublicDir, bak.PublicDir)
	add("container port", portText(cur.ContainerPort), portText(bak.ContainerPort))
	add("host port", portText(cur.HostPort), portText(bak.HostPort))
	return out
}

// runtimeName spells the empty runtime as what it actually means, so a move
// between the shared pool and a per-site container reads as one.
func runtimeName(r string) string {
	if r == "" {
		return "fpm"
	}
	return r
}

// portText renders an unset port as empty so add() treats two unset ports as
// unchanged rather than reporting "0 → 0".
func portText(p int) string {
	if p == 0 {
		return ""
	}
	return strconv.Itoa(p)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
