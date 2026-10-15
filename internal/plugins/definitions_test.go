package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func writeDefinition(t *testing.T, pluginDir, rel, body string) {
	t.Helper()
	path := filepath.Join(pluginDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefinitionDirs_OnlyFromPluginsThatLoad(t *testing.T) {
	isolate(t)
	good := writePlugin(t, Dir(), "good", "api_version: 1\nname: good\n")
	writeDefinition(t, good, "services/queuebox.yaml", "name: queuebox\nimage: docker.io/x/q:1\n")
	bad := writePlugin(t, Dir(), "bad", "api_version: 9\nname: bad\n")
	writeDefinition(t, bad, "services/other.yaml", "name: other\nimage: docker.io/x/o:1\n")

	got := config.PluginDefinitionDirs(config.PluginServices)
	if len(got) != 1 || got[0] != filepath.Join(good, "services") {
		t.Errorf("dirs = %v, want only the loading plugin's", got)
	}
	if !config.PresetExists("queuebox") || config.PresetExists("other") {
		t.Error("the service picker must offer the loading plugin's preset and not the broken one's")
	}
}

func TestReadPlugin_RefusesABrokenDefinition(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\n")
	writeDefinition(t, dir, "frameworks/acme@2.yaml", "name: acme\n")
	if _, err := ReadPlugin(dir); err == nil || !strings.Contains(err.Error(), "acme@2") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstall_RefusesANameLerdAlreadyHas(t *testing.T) {
	isolate(t)
	src := writePlugin(t, t.TempDir(), "p", "api_version: 1\nname: p\n")
	writeDefinition(t, src, "services/mysql.yaml", "name: mysql\nimage: docker.io/x/mysql:1\n")
	_, err := Install(src)
	if err == nil || !strings.Contains(err.Error(), "service mysql") {
		t.Fatalf("err = %v, want the clash named", err)
	}
	if _, err := os.Stat(filepath.Join(Dir(), "p")); !os.IsNotExist(err) {
		t.Error("a refused plugin must leave nothing installed")
	}
}

func TestUpdateListed_KeepsItsOwnDefinitions(t *testing.T) {
	isolate(t)
	repo, _ := gitRepo(t, "hello")
	writeDefinition(t, repo, "services/queuebox.yaml", "name: queuebox\nimage: docker.io/x/q:1\n")
	commit := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit("add", ".")
	commit("commit", "-q", "-m", "ships a service")
	first := commit("rev-parse", "HEAD")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: first})
	if _, err := InstallListed("hello"); err != nil {
		t.Fatal(err)
	}
	writeDefinition(t, repo, "services/queuebox.yaml", "name: queuebox\nimage: docker.io/x/q:2\n")
	commit("commit", "-q", "-am", "newer service")
	serveIndex(t, Listing{Name: "hello", Repo: "file://" + repo, Commit: commit("rev-parse", "HEAD")})
	if err := refreshCatalog(); err != nil {
		t.Fatal(err)
	}
	if err := UpdateListed("hello"); err != nil {
		t.Fatalf("an update re-ships the plugin's own names: %v", err)
	}
}
