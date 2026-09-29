package cli

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func fwWithServices() *config.Framework {
	return &config.Framework{Env: config.FrameworkEnvConf{Services: map[string]config.FrameworkServiceDef{
		"redis": {Vars: []string{"REDIS_HOST=lerd-redis", "REDIS_PORT=6379"}},
		"mysql": {Vars: []string{"DB_HOST=lerd-mysql", "DB_PORT=3306", "DB_DATABASE={{site}}"}},
	}}}
}

// A worktree created before the runtime switch kept the container names its
// parent has since had rewritten to loopback. Neither key equals the parent's
// previous value any more, so the carry that follows a parent's change walks
// past both and the worktree keeps connecting to a name that resolves nowhere.
func TestStaleServiceCarry(t *testing.T) {
	fw := fwWithServices()
	after := map[string]string{
		"REDIS_HOST": "127.0.0.1", "REDIS_PORT": "6380",
		"DB_HOST": "127.0.0.1", "DB_PORT": "3308", "DB_DATABASE": "parkapp",
	}

	t.Run("a container name takes the whole service block", func(t *testing.T) {
		wt := map[string]string{
			"REDIS_HOST": "lerd-redis", "REDIS_PORT": "6379",
			"DB_HOST": "127.0.0.1", "DB_PORT": "3308", "DB_DATABASE": "parkapp_feature",
		}
		got := staleServiceCarry(fw, wt, after)
		// The port travels with the host: fixing one and not the other leaves
		// the worktree pointed at loopback on the container's internal port.
		if got["REDIS_HOST"] != "127.0.0.1" || got["REDIS_PORT"] != "6380" {
			t.Errorf("redis = %v, want the parent's host and port", got)
		}
		// mysql names no container here, so its block is left alone and the
		// worktree keeps the database it was given.
		if _, ok := got["DB_DATABASE"]; ok {
			t.Errorf("carried %q from a service the worktree does not name by container", "DB_DATABASE")
		}
	})

	t.Run("a worktree already on loopback is left alone", func(t *testing.T) {
		wt := map[string]string{
			"REDIS_HOST": "127.0.0.1", "REDIS_PORT": "6380",
			"DB_HOST": "127.0.0.1", "DB_PORT": "3308", "DB_DATABASE": "parkapp_feature",
		}
		if got := staleServiceCarry(fw, wt, after); len(got) != 0 {
			t.Errorf("carried %v, want nothing", got)
		}
	})

	t.Run("a container runtime carries nothing, both sides name it", func(t *testing.T) {
		container := map[string]string{"REDIS_HOST": "lerd-redis", "REDIS_PORT": "6379"}
		if got := staleServiceCarry(fw, container, container); len(got) != 0 {
			t.Errorf("carried %v, want nothing when the parent names it too", got)
		}
	})

	t.Run("a database named after the service is not a container reference", func(t *testing.T) {
		wt := map[string]string{"DB_HOST": "127.0.0.1", "DB_PORT": "3308", "DB_DATABASE": "lerd-stats"}
		if got := staleServiceCarry(fw, wt, after); len(got) != 0 {
			t.Errorf("carried %v, want nothing for a database that merely starts with lerd-", got)
		}
	})
}

func TestStaleServiceCarry_NoFramework(t *testing.T) {
	if got := staleServiceCarry(nil, map[string]string{"REDIS_HOST": "lerd-redis"}, map[string]string{"REDIS_HOST": "127.0.0.1"}); len(got) != 0 {
		t.Errorf("carried %v with no framework, want nothing", got)
	}
}

func TestStaleServiceCarry_KeysAreServiceScoped(t *testing.T) {
	fw := fwWithServices()
	wt := map[string]string{"REDIS_HOST": "lerd-redis", "REDIS_PORT": "6379"}
	after := map[string]string{"REDIS_HOST": "127.0.0.1", "REDIS_PORT": "6380"}
	got := staleServiceCarry(fw, wt, after)
	for k := range got {
		if !strings.HasPrefix(k, "REDIS_") {
			t.Errorf("carried %q, want only the redis block", k)
		}
	}
}

// An isolated worktree owns its database, and the store says so by declaring
// that var with a placeholder. Carrying a stale service block must not hand it
// the parent's, which is the whole reason lerd env stopped being run in a
// worktree in the first place.
func TestStaleServiceCarry_LeavesAWorktreeItsOwnDatabase(t *testing.T) {
	fw := fwWithServices()
	wt := map[string]string{
		"DB_HOST": "lerd-mysql", "DB_PORT": "3306", "DB_DATABASE": "parkapp_feature",
	}
	after := map[string]string{
		"DB_HOST": "127.0.0.1", "DB_PORT": "3308", "DB_DATABASE": "parkapp",
	}
	got := staleServiceCarry(fw, wt, after)
	if got["DB_HOST"] != "127.0.0.1" || got["DB_PORT"] != "3308" {
		t.Errorf("server coordinates = %v, want the parent's host and port", got)
	}
	if v, ok := got["DB_DATABASE"]; ok {
		t.Errorf("carried DB_DATABASE=%q, want the worktree to keep its own", v)
	}
}
