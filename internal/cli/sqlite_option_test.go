package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Which databases a framework can use is the definition's to declare, like
// everything else about how it is wired. Offering SQLite to a framework that
// cannot use it is how a project ends up picking a database its application
// will never open.
func TestBuildDatabaseOptions_offersSQLiteOnlyWhereDeclared(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	declares := &config.Framework{Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
		"sqlite": {Vars: []string{"DB_CONNECTION=sqlite"}},
		"mysql":  {Vars: []string{"DB_HOST=lerd-mysql"}},
	}}}
	_, names := buildDatabaseOptions(declares)
	if !names["sqlite"] {
		t.Error("a framework declaring sqlite was not offered it")
	}

	declaresNot := &config.Framework{Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
		"mysql": {Vars: []string{"DB_HOST=lerd-mysql"}},
	}}}
	if _, names := buildDatabaseOptions(declaresNot); names["sqlite"] {
		t.Error("a framework that declares no sqlite service was offered it anyway")
	}
}

// A project lerd recognises no framework for keeps the option: nothing has
// declared otherwise, and a file database is a reasonable answer for it.
func TestBuildDatabaseOptions_keepsSQLiteWithoutAFramework(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if _, names := buildDatabaseOptions(nil); !names["sqlite"] {
		t.Error("a project with no framework was not offered sqlite")
	}
	empty := &config.Framework{}
	if _, names := buildDatabaseOptions(empty); !names["sqlite"] {
		t.Error("a framework declaring no env services at all was not offered sqlite")
	}
}
