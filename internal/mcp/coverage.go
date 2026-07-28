package mcp

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	lerdNode "github.com/geodro/lerd/internal/node"
	"github.com/geodro/lerd/internal/serviceops"
)

// dbEnvForArgs resolves the project's database the same way the other db
// actions do: the path argument (or the injected cwd site), with an explicit
// database overriding what the env file names. Returns a tool error to hand back
// verbatim when it cannot.
func dbEnvForArgs(args map[string]any) (*mcpDBEnv, any) {
	projectPath := resolvedPath(args)
	if projectPath == "" {
		return nil, toolErr("path is required — pass a path argument or open your assistant in the project directory")
	}
	env, err := readDBEnv(projectPath)
	if err != nil {
		return nil, toolErr(err.Error())
	}
	if db := strArg(args, "database"); db != "" {
		env.database = db
	}
	return env, nil
}

// execDBExtensionList reports the extensions an engine's image can create and
// which of them the database already has. lerd creates one on its own when an
// import reaches for a type it provides, so this is for seeing the set and for
// adding one before any dump arrives.
func execDBExtensionList(args map[string]any) (any, *rpcError) {
	env, rerr := dbEnvForArgs(args)
	if rerr != nil {
		return rerr, nil
	}
	svc := mcpDBService(env)
	installed, err := serviceops.InstalledExtensions(svc, env.database)
	if err != nil {
		return toolErr(fmt.Sprintf("listing extensions in %s: %v", env.database, err)), nil
	}
	have := map[string]bool{}
	for _, name := range installed {
		have[name] = true
	}
	available := []map[string]any{}
	for _, e := range serviceops.DeclaredExtensions(svc) {
		if have[e.Name] {
			continue
		}
		available = append(available, map[string]any{"name": e.Name, "types": e.Types, "always": e.Always})
	}
	return toolJSON(map[string]any{
		"service": svc, "database": env.database,
		"installed": installed, "available": available,
	}), nil
}

// execDBExtensionAdd creates one declared extension in the database.
func execDBExtensionAdd(args map[string]any) (any, *rpcError) {
	name := strArg(args, "extension")
	if name == "" {
		return toolErr("extension is required"), nil
	}
	env, rerr := dbEnvForArgs(args)
	if rerr != nil {
		return rerr, nil
	}
	svc := mcpDBService(env)
	if err := serviceops.CreateExtension(svc, env.database, name); err != nil {
		return toolErr(err.Error()), nil
	}
	return toolOK(fmt.Sprintf("extension %s ready in %s", name, env.database)), nil
}

// execServiceEntities lists what a service holds beyond databases: the kinds its
// preset declares (buckets and anything added later), their columns, the actions
// each supports, and the current rows. Databases keep their own tool.
func execServiceEntities(args map[string]any) (any, *rpcError) {
	name := strArg(args, "name")
	if name == "" {
		return toolErr("name is required"), nil
	}
	specs := serviceops.ServiceEntities(name)
	if len(specs) == 0 {
		return toolErr(fmt.Sprintf("%s declares no entities beyond databases; use the db tool for those", name)), nil
	}
	kinds := []map[string]any{}
	for i := range specs {
		spec := &specs[i]
		entry := map[string]any{"kind": spec.Kind, "label": spec.Label, "actions": entityActionNames(spec)}
		rows, err := serviceops.ListEntities(name, spec)
		if err != nil {
			entry["error"] = err.Error()
		} else {
			listed := []map[string]any{}
			for _, r := range rows {
				listed = append(listed, map[string]any{"name": r.Name, "values": r.Values})
			}
			entry["rows"] = listed
		}
		kinds = append(kinds, entry)
	}
	return toolJSON(map[string]any{"service": name, "entities": kinds}), nil
}

// execServiceEntityAction runs one declared action against a named entity.
// export and import stream a file, so they stay on the CLI and the dashboard.
func execServiceEntityAction(args map[string]any) (any, *rpcError) {
	name, kind := strArg(args, "name"), strArg(args, "kind")
	entity, action := strArg(args, "entity"), strArg(args, "entity_action")
	switch {
	case name == "":
		return toolErr("name is required"), nil
	case kind == "":
		return toolErr("kind is required"), nil
	case entity == "":
		return toolErr("entity is required"), nil
	case action == "":
		return toolErr("entity_action is required"), nil
	case action == "export" || action == "import":
		return toolErr("export and import stream a file; run them from the CLI or the dashboard"), nil
	}
	spec := serviceops.EntityFor(name, kind)
	if spec == nil {
		return toolErr(fmt.Sprintf("%s declares no %s", name, kind)), nil
	}
	if err := serviceops.RunEntityAction(name, spec, action, entity); err != nil {
		return toolErr(err.Error()), nil
	}
	return toolOK(fmt.Sprintf("%s %s on %s in %s", action, kind, entity, name)), nil
}

// entityActionNames lists a kind's declared actions in a stable order.
func entityActionNames(spec *config.EntitySpec) []string {
	out := make([]string, 0, len(spec.Actions))
	for name := range spec.Actions {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// execNodeManager reports the Node version manager lerd drives, or switches it.
// The switch runs through the CLI because it also rewrites the PATH shims and
// regenerates host worker units, which live there.
func execNodeManager(args map[string]any) (any, *rpcError) {
	target := strArg(args, "manager")
	cfg, err := config.LoadGlobal()
	if err != nil || cfg == nil {
		return toolErr("loading config failed"), nil
	}
	current := cfg.NodeManager()
	if target == "" {
		return toolJSON(map[string]any{
			"manager":       current,
			"nvm_available": lerdNode.ManagerByName("nvm").Available(),
			"managed":       lerdNode.Managed(),
		}), nil
	}
	if target != "fnm" && target != "nvm" {
		return toolErr(fmt.Sprintf("unknown manager %q: use fnm or nvm", target)), nil
	}
	if target == current {
		return toolOK("already using " + target), nil
	}
	cwd, _ := os.Getwd()
	out, err := runIn(cwd, lerdSelf(), "node:manager", target)
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return toolErr(strings.TrimSpace(stripANSI(out))), nil
	}
	return toolOK("Node version manager set to " + target), nil
}
