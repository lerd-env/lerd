package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const providerManifest = "api_version: 1\nname: %s\nprovides:\n  worktree_database: bin/run\n"

func providerPlugin(t *testing.T, root, name, script string) string {
	t.Helper()
	dir := writePlugin(t, root, name, strings.Replace(providerManifest, "%s", name, 1))
	if script != "" {
		if err := os.WriteFile(filepath.Join(dir, "bin", "run"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadPlugin_RefusesAJobLerdDoesNotHave(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\nprovides:\n  dns: bin/run\n")
	if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), `"dns"`) {
		t.Fatalf("err = %v, want the unknown job named", err)
	}
}

func TestProviderOf(t *testing.T) {
	isolate(t)
	if ProviderOf(JobWorktreeDatabase) != nil {
		t.Fatal("with no plugin in charge, lerd does the job itself")
	}
	providerPlugin(t, Dir(), "brancher", "")
	if p := ProviderOf(JobWorktreeDatabase); p == nil || p.Name != "brancher" {
		t.Fatalf("got %v", p)
	}
}

// Two plugins in charge of one job would each assume the other is not there.
func TestInstall_RefusesASecondPluginForTheSameJob(t *testing.T) {
	isolate(t)
	providerPlugin(t, Dir(), "brancher", "")
	src := providerPlugin(t, t.TempDir(), "rival", "")
	if _, err := Install(src); err == nil || !strings.Contains(err.Error(), "brancher") || !strings.Contains(err.Error(), JobWorktreeDatabase) {
		t.Fatalf("err = %v, want the plugin already in charge named", err)
	}
	stage := t.TempDir()
	staged := filepath.Join(stage, "plugin")
	if err := os.Rename(providerPlugin(t, stage, "brancher", ""), staged); err != nil {
		t.Fatal(err)
	}
	if _, err := place(stage, staged, "", true, nil); err != nil {
		t.Fatalf("updating the plugin in charge is not a clash: %v", err)
	}
}

func TestProvide_PassesTheActionAndRequestAndReturnsTheAnswer(t *testing.T) {
	isolate(t)
	providerPlugin(t, Dir(), "brancher", "#!/bin/sh\nread -r body\necho \"{\\\"action\\\":\\\"$1\\\",\\\"got\\\":$body}\"\n")
	out, err := ProviderOf(JobWorktreeDatabase).Provide(JobWorktreeDatabase, "create", map[string]string{"branch": "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != `{"action":"create","got":{"branch":"feature"}}` {
		t.Errorf("answer = %s", got)
	}
}

func TestProvide_FailureCarriesWhatThePluginSaid(t *testing.T) {
	isolate(t)
	providerPlugin(t, Dir(), "brancher", "#!/bin/sh\necho 'no space left for a branch' >&2\nexit 3\n")
	_, err := ProviderOf(JobWorktreeDatabase).Provide(JobWorktreeDatabase, "create", nil)
	if err == nil || !strings.Contains(err.Error(), "no space left for a branch") {
		t.Fatalf("err = %v", err)
	}
	if f, ok := LastFailure("brancher"); !ok || f.What != JobWorktreeDatabase+" create" {
		t.Errorf("recorded %+v %v", f, ok)
	}
}
