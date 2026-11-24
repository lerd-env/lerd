package cli

import (
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// keepForeignDBFlag is what every `lerd env` lerd starts on its own carries. A run
// the user asked for, or one that is moving the database, goes without it.
const keepForeignDBFlag = "keep-foreign-db"

var envKeepForeignDB bool

// keptDB is the database connection a run leaves as the project has it.
type keptDB struct {
	host string
	keys []string
}

// foreignDBToKeep is the connection this run must not rewrite, nil when it may.
func foreignDBToKeep(dir string, keep bool) *keptDB {
	if !keep {
		return nil
	}
	host, foreign := config.ForeignDBHost(dir)
	if !foreign {
		return nil
	}
	return &keptDB{host: host, keys: connectionKeys(config.DBEnvBindingFor(dir))}
}

// connectionKeys are the env keys naming the server a project's database is on.
func connectionKeys(b config.DBEnvBinding) []string {
	return slices.DeleteFunc([]string{b.HostKey, b.URLKey}, func(k string) bool { return k == "" })
}

// writtenBy reports whether a service's vars would replace the kept connection.
func (k *keptDB) writtenBy(vars []string) bool {
	if k == nil {
		return false
	}
	return slices.ContainsFunc(vars, func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.Contains(k.keys, key)
	})
}

// filledBy reports whether a service a project declares would take the place of
// the kept connection, a drop-in counting as the service it stands in for.
func (k *keptDB) filledBy(fw *config.Framework, svc config.ProjectService) bool {
	if k == nil {
		return false
	}
	role, ok := frameworkServiceRole(fw, &config.CustomService{Name: svc.Name, Preset: svc.Preset, Family: config.FamilyOfName(svc.Name)})
	return ok && k.writtenBy(fw.Env.Services[role].Vars)
}

// pinned adds the kept connection to the keys a run hands to no worktree: one
// that left the parent's server alone moves no checkout onto it either.
func (k *keptDB) pinned(fw *config.Framework, overrides map[string]string) map[string]string {
	if k == nil {
		return overrides
	}
	out := maps.Clone(overrides)
	if out == nil {
		out = map[string]string{}
	}
	for _, key := range worktreeDBConnectionKeys {
		out[key] = ""
	}
	if fw == nil {
		return out
	}
	for _, def := range fw.Env.Services {
		if !k.writtenBy(def.Vars) {
			continue
		}
		for _, kv := range def.Vars {
			key, _, _ := strings.Cut(kv, "=")
			out[key] = ""
		}
	}
	return out
}

// worktreeLerdDB is what a checkout on lerd's database needs to keep reaching it
// when its parent's connection is kept and so has nothing to hand on. The service
// it is on and the database it names stay its own.
func worktreeLerdDB(site *config.Site, fw *config.Framework, dir string, vals map[string]string) map[string]string {
	host := config.DBEndpointFor(dir).Host
	if fw == nil || host == "" {
		return nil
	}
	binding := config.DBEnvBindingFor(dir)
	for role, def := range fw.Env.Services {
		if !frameworkServiceDetected(def, vals) || !(&keptDB{keys: connectionKeys(binding)}).writtenBy(def.Vars) {
			continue
		}
		svc := config.DBServiceFor(dir, host)
		if svc == "" {
			svc = role
		}
		updates := map[string]string{}
		for _, kv := range frameworkVarsForAlternate(fw, role, &config.CustomService{Name: svc}) {
			k, v, _ := strings.Cut(kv, "=")
			if k == binding.URLKey || !strings.Contains(v, "{{") {
				updates[k] = v
			}
		}
		if usesLoopbackServices(site) {
			rewriteEnvForHostProxy(updates, loopbackServiceNames(map[string]bool{svc: true}, knownServices()))
		}
		if url, ok := updates[binding.URLKey]; ok {
			updates[binding.URLKey] = config.DBURLWithName(url, config.DBNameInURL(vals[binding.URLKey]))
		}
		maps.DeleteFunc(updates, func(k, v string) bool { return strings.Trim(vals[k], `"'`) == v })
		return updates
	}
	return nil
}

// runLerdEnvKeepingDB is the `lerd env` behind a change lerd makes on its own.
func runLerdEnvKeepingDB(dir string) error {
	return runLerdEnvKeepingDBTo(dir, os.Stdout)
}

func runLerdEnvKeepingDBTo(dir string, out io.Writer) error {
	return runLerdEnvTo(dir, out, "--"+keepForeignDBFlag)
}

// announceEnvSweep names the sites whose env is about to be rewritten, and the
// ones whose database connection stays where it is.
func announceEnvSweep(sites []config.Site) {
	var names []string
	for _, s := range sites {
		if frameworkManagesEnv(s.Path) {
			names = append(names, s.Name)
		}
	}
	if len(names) > 0 {
		feedback.Note("rewriting .env for " + strings.Join(names, ", "))
	}
	announceKeptDBs(sites)
}

func announceKeptDBs(sites []config.Site) {
	var kept []string
	for _, s := range sites {
		if !frameworkManagesEnv(s.Path) {
			continue
		}
		if host, foreign := config.ForeignDBHost(s.Path); foreign {
			kept = append(kept, s.Name+" ("+host+")")
		}
		worktrees, _ := switchWorktreesFor(s.Path, s.PrimaryDomain())
		for _, wt := range worktrees {
			if host, foreign := config.ForeignDBHost(wt.Path); foreign {
				kept = append(kept, s.Name+"/"+wt.Branch+" ("+host+")")
			}
		}
	}
	if len(kept) > 0 {
		feedback.Note("keeping the database connection of " + strings.Join(kept, ", ") + ", it is not one of lerd's. Run `lerd env` in a site to move it to lerd")
	}
}
