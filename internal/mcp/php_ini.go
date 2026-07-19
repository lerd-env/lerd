package mcp

import (
	"fmt"

	"github.com/geodro/lerd/internal/cfgedit"
	"github.com/geodro/lerd/internal/phpini"
)

// iniScope resolves the php.ini editor scope from the tool args: the shared file
// when `shared` is true or `version` is "shared"/empty, otherwise the given PHP
// version. It validates the scope so an unknown version or non-FrankenPHP site
// is rejected before any disk write.
func iniScope(args map[string]any) (string, error) {
	scope := strArg(args, "version")
	if boolArg(args, "shared") || scope == "" || scope == phpini.SharedScope {
		scope = phpini.SharedScope
	}
	if !phpini.Valid(scope) {
		return "", fmt.Errorf("invalid php.ini scope %q, pass an installed PHP version, or shared=true for the shared file", scope)
	}
	return scope, nil
}

func execPHPIniRead(args map[string]any) (any, *rpcError) {
	scope, err := iniScope(args)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	got, err := phpini.ScopeFile(scope).Read()
	if err != nil {
		return toolErr(err.Error()), nil
	}
	state := "saved override"
	if !got.Exists {
		state = "no override yet, showing the template"
	}
	return toolOK(fmt.Sprintf("# %s php.ini (%s)\n%s", scope, state, got.Body)), nil
}

func execPHPIniWrite(args map[string]any) (any, *rpcError) {
	scope, err := iniScope(args)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	content := strArg(args, "content")
	// Seed the bind-mount source first so a first-ever write can't race podman
	// into auto-creating a directory at the conf.d path.
	if err := phpini.Ensure(scope); err != nil {
		return toolErr(err.Error()), nil
	}
	res, err := phpini.ScopeFile(scope).Save(content, cfgedit.SaveOpts{
		Backup: true,
		Apply:  func() error { return phpini.Restart(scope) },
	})
	if err != nil {
		return toolErr(err.Error()), nil
	}
	if !res.OK {
		return toolErr(res.Error), nil
	}
	return toolOK(fmt.Sprintf("Saved %s php.ini and restarted the affected FPM containers.", scope)), nil
}

func execPHPIniReset(args map[string]any) (any, *rpcError) {
	scope, err := iniScope(args)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	if err := phpini.ScopeFile(scope).Reset(func() error { return phpini.RestartNoSeed(scope) }); err != nil {
		return toolErr(err.Error()), nil
	}
	return toolOK(fmt.Sprintf("Reset %s php.ini to the bundled defaults.", scope)), nil
}
