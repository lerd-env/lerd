package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func drupalishEnv(def string) *config.Framework {
	driver := []config.FrameworkServiceDetect{{Key: "databases.default.default.driver", ValuePrefix: "mysql"}}
	pg := []config.FrameworkServiceDetect{{Key: "databases.default.default.driver", ValuePrefix: "pgsql"}}
	lite := []config.FrameworkServiceDetect{{Key: "databases.default.default.driver", ValuePrefix: "sqlite"}}
	return &config.Framework{Name: "drupalish", Env: config.FrameworkEnvConf{
		DefaultService: def,
		SQLite:         &config.FrameworkServiceDef{Detect: lite},
		Services: map[string]config.FrameworkServiceDef{
			"mysql":    {Detect: driver},
			"postgres": {Detect: pg},
		},
	}}
}

// A fresh scaffold names no database at all, so nothing is detected and the
// install step has nothing to run against. The definition's default fills it.
func TestDefaultServiceFor_freshProjectGetsTheDefault(t *testing.T) {
	got, err := defaultServiceFor(drupalishEnv("mysql"), map[string]string{}, map[string]bool{}, map[string]bool{})
	if err != nil || got != "mysql" {
		t.Errorf("defaultServiceFor = %q, %v; want mysql", got, err)
	}
}

// A project already on another engine, or on a file, is never rewired.
func TestDefaultServiceFor_leavesAConfiguredProjectAlone(t *testing.T) {
	fw := drupalishEnv("mysql")
	for _, driver := range []string{"pgsql", "sqlite"} {
		env := map[string]string{"databases.default.default.driver": driver}
		if got, err := defaultServiceFor(fw, env, map[string]bool{}, map[string]bool{}); err != nil || got != "" {
			t.Errorf("driver %s: defaultServiceFor = %q, %v; want nothing", driver, got, err)
		}
	}
	if got, _ := defaultServiceFor(fw, map[string]string{}, map[string]bool{"postgres": true}, map[string]bool{}); got != "" {
		t.Errorf("a picked postgres got the default %q", got)
	}
	if got, _ := defaultServiceFor(fw, map[string]string{}, map[string]bool{}, map[string]bool{"mysql": true}); got != "" {
		t.Errorf("an external database got the default %q", got)
	}
}

func TestDefaultServiceFor_unsetIsANoop(t *testing.T) {
	if got, err := defaultServiceFor(drupalishEnv(""), map[string]string{}, map[string]bool{}, map[string]bool{}); err != nil || got != "" {
		t.Errorf("defaultServiceFor = %q, %v; want nothing", got, err)
	}
}

// A value this binary cannot wire is refused by name, never guessed at.
func TestDefaultServiceFor_refusesAServiceTheDefinitionDoesNotWire(t *testing.T) {
	_, err := defaultServiceFor(drupalishEnv("cockroach"), map[string]string{}, map[string]bool{}, map[string]bool{})
	if err == nil || !strings.Contains(err.Error(), "cockroach") {
		t.Errorf("err = %v, want a refusal naming cockroach", err)
	}
}
