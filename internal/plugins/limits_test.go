package plugins

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunWithin_StopsAScriptThatOverruns(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 5")
	start := time.Now()
	err := RunWithin(cmd, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out after 100ms") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("the script must be stopped at its limit, not waited for")
	}
	if err := RunWithin(exec.Command("true"), time.Second); err != nil {
		t.Errorf("a quick script: %v", err)
	}
}

func TestEntryLimits(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", `api_version: 1
name: p
site_commands:
  - {name: long, label: Long, run: bin/run, timeout: 45m}
  - {name: plain, label: Plain, run: bin/run}
mcp:
  tools:
    - {name: t, description: d, run: bin/run, timeout: 5m}
api:
  - {path: x, run: bin/run}
`)
	p, err := ReadPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.SiteCommands[0].Limit(); got != 45*time.Minute {
		t.Errorf("declared site command limit %v", got)
	}
	if got := p.SiteCommands[1].Limit(); got != SiteCommandLimit {
		t.Errorf("default site command limit %v", got)
	}
	if got := p.MCP.Tools[0].Limit(); got != 5*time.Minute {
		t.Errorf("mcp limit %v", got)
	}
	if got := p.API[0].Limit(); got != APILimit {
		t.Errorf("api limit %v", got)
	}
}

func TestReadPlugin_RefusesATimeoutPastItsCeiling(t *testing.T) {
	cases := map[string]string{
		"site_commands:\n  - {name: a, label: A, run: bin/run, timeout: 2h}\n":         "1h0m0s",
		"api:\n  - {path: x, run: bin/run, timeout: 11m}\n":                            "10m0s",
		"mcp:\n  tools:\n    - {name: t, description: d, run: bin/run, timeout: 1d}\n": `"1d"`,
		"api:\n  - {path: x, run: bin/run, timeout: -1s}\n":                            `"-1s"`,
	}
	for body, want := range cases {
		dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\n"+body)
		if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to name %s", body, err, want)
		}
	}
}

func TestEmit_HookThatOverrunsIsStoppedAndRecorded(t *testing.T) {
	isolate(t)
	orig := hookLimit
	hookLimit = 100 * time.Millisecond
	t.Cleanup(func() { hookLimit = orig })
	hookPlugin(t, "slow", EventSiteLinked, "#!/bin/sh\nsleep 5\n")
	Emit(EventSiteLinked, map[string]string{"site": "shop"})
	if f, ok := LastFailure("slow"); !ok || !strings.Contains(f.Error, "timed out") {
		t.Errorf("recorded %+v %v", f, ok)
	}
}
