package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runningAs(t *testing.T, v string) {
	t.Helper()
	orig := runningVersion
	runningVersion = v
	t.Cleanup(func() { runningVersion = orig })
}

func TestCompatible(t *testing.T) {
	cases := []struct {
		running, constraint string
		want                bool
	}{
		{"1.37.0", "", true},
		{"1.37.0", ">=1.37", true},
		{"1.38.2", ">=1.37", true},
		{"2.0.0", ">=1.37", true},
		{"1.36.4", ">=1.37", false},
		{"1.37.0-beta.3-5-gabc1234", ">=1.37", true},
		{"1.37.0", ">= 1.37.2", true},
		{"1.10.0", ">=1.9", true},
		// Only a lower bound is a promise lerd can keep, anything else is refused.
		{"1.37.0", "^1.37", false},
		{"1.37.0", "<2.0", false},
		// A build from a commit with no tag behind it has no version to judge.
		{"d26efaf9", ">=9.0", true},
	}
	for _, c := range cases {
		runningAs(t, c.running)
		if got := Compatible(c.constraint); got != c.want {
			t.Errorf("lerd %s against %q = %v, want %v", c.running, c.constraint, got, c.want)
		}
	}
}

func TestReadPlugin_RefusesANewerLerdByName(t *testing.T) {
	runningAs(t, "1.36.0")
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\nlerd: \">=1.37\"\n")
	_, err := ReadPlugin(dir)
	if err == nil || !strings.Contains(err.Error(), ">=1.37") || !strings.Contains(err.Error(), "1.36.0") {
		t.Fatalf("err = %v, want it to name the requirement and the running version", err)
	}
}

func TestPendingUpdate_SkipsACommitThatNeedsANewerLerd(t *testing.T) {
	isolate(t)
	repo, approved := gitRepo(t, "hello")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: approved})
	if _, err := InstallListed("hello"); err != nil {
		t.Fatal(err)
	}
	runningAs(t, "1.36.0")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: strings.Repeat("b", 40), Lerd: ">=1.37"})
	if err := refreshCatalog(); err != nil {
		t.Fatal(err)
	}

	if l, ok := PendingUpdate("hello"); ok {
		t.Errorf("an update this lerd cannot load must not be offered, got %+v", l)
	}
}

// A new plugin is written against the lerd that scaffolded it, so that is the
// oldest it can promise to run on.
func TestScaffold_RequiresTheLerdThatWroteIt(t *testing.T) {
	for running, want := range map[string]string{"1.38.2": "lerd: \">=1.38\"\n", "1.38.0-beta.1-3-gabc": "lerd: \">=1.38\"\n", "d26efaf9": ""} {
		runningAs(t, running)
		dir := filepath.Join(t.TempDir(), "hi")
		if err := Scaffold("hi", dir); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, ManifestFile))
		got := strings.Contains(string(data), "\nlerd:")
		if got != (want != "") || (want != "" && !strings.Contains(string(data), "\nname: hi\n"+want)) {
			t.Errorf("lerd %s scaffolded:\n%s", running, data)
		}
	}
}
