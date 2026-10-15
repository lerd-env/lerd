package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hookPlugin installs a plugin whose bin/run records its env and stdin, hooked
// to the given event.
func hookPlugin(t *testing.T, name, event, script string) {
	t.Helper()
	dir := writePlugin(t, Dir(), name, "api_version: 1\nname: "+name+"\nhooks:\n  "+event+": bin/run\n")
	if err := os.WriteFile(filepath.Join(dir, "bin", "run"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestEmit_RunsHookWithPayload(t *testing.T) {
	isolate(t)
	hookPlugin(t, "rec", EventSiteLinked, "#!/bin/sh\necho \"$LERD_EVENT|$LERD_SURFACE|$LERD_SITE\" > \"$LERD_PLUGIN_DATA/env\"\ncat > \"$LERD_PLUGIN_DATA/stdin\"\n")

	Emit(EventSiteLinked, map[string]string{"site": "shop", "domain": "shop.test"})

	env, err := os.ReadFile(filepath.Join(DataDirFor("rec"), "env"))
	if err != nil {
		t.Fatalf("hook did not run: %v", err)
	}
	if got := strings.TrimSpace(string(env)); got != "site.linked|hook|shop" {
		t.Errorf("env = %q", got)
	}
	var payload map[string]string
	stdin, _ := os.ReadFile(filepath.Join(DataDirFor("rec"), "stdin"))
	if err := json.Unmarshal(stdin, &payload); err != nil {
		t.Fatalf("stdin is not JSON: %q", stdin)
	}
	if payload["event"] != "site.linked" || payload["site"] != "shop" || payload["domain"] != "shop.test" {
		t.Errorf("payload = %v", payload)
	}
}

func TestEmit_SkipsOtherEvents(t *testing.T) {
	isolate(t)
	hookPlugin(t, "rec", EventSiteUnlinked, "#!/bin/sh\ntouch \"$LERD_PLUGIN_DATA/ran\"\n")

	Emit(EventSiteLinked, map[string]string{"site": "shop"})

	if _, err := os.Stat(filepath.Join(DataDirFor("rec"), "ran")); err == nil {
		t.Error("a hook must only run for its own event")
	}
}

// A failing hook is the plugin's problem: it is logged and the next plugin still runs.
func TestEmit_FailingHookIsLoggedAndOthersStillRun(t *testing.T) {
	isolate(t)
	hookPlugin(t, "a-broken", EventSiteLinked, "#!/bin/sh\necho boom >&2\nexit 3\n")
	hookPlugin(t, "b-fine", EventSiteLinked, "#!/bin/sh\ntouch \"$LERD_PLUGIN_DATA/ran\"\n")

	Emit(EventSiteLinked, map[string]string{"site": "shop"})

	if _, err := os.Stat(filepath.Join(DataDirFor("b-fine"), "ran")); err != nil {
		t.Error("a broken plugin must not stop the next one")
	}
	log, _ := os.ReadFile(filepath.Join(DataDirFor("a-broken"), HookLog))
	if !strings.Contains(string(log), "boom") || !strings.Contains(string(log), "exit status 3") {
		t.Errorf("hook log should hold the output and the failure, got %q", log)
	}
	if f, ok := LastFailure("a-broken"); !ok || f.Surface != SurfaceHook || f.What != EventSiteLinked || !strings.Contains(f.Error, "boom") {
		t.Errorf("a failing hook is recorded so the dashboard can offer a report, got %+v %v", f, ok)
	}
}

func TestEmit_UnknownEventPanics(t *testing.T) {
	isolate(t)
	defer func() {
		if recover() == nil {
			t.Error("emitting an undeclared event is a programming error")
		}
	}()
	Emit("site.nope", nil)
}
