package linker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// A project that declares no domains gets the one the link derives from its
// directory, so writing it back only dirtied the committed file.
func TestSyncDomains_leavesADerivedDomainOutOfTheProject(t *testing.T) {
	dir := projectDir(t, "demo", "php_version: \"8.4\"\n")
	p := CLIPolicy("", false, nil)
	plan, err := Resolve(dir, testConfig(), p)
	if err != nil {
		t.Fatal(err)
	}

	syncDomains(plan, plan.Site, p, "test")

	if got := readFile(t, filepath.Join(dir, ".lerd.yaml")); strings.Contains(got, "domains") {
		t.Errorf(".lerd.yaml = %q, want no domains written for the derived one", got)
	}
}

// A project that does declare domains keeps them in step with the registry.
func TestSyncDomains_keepsDeclaredDomainsInStep(t *testing.T) {
	dir := projectDir(t, "demo", "domains:\n  - demo\n")
	p := CLIPolicy("", false, nil)
	plan, err := Resolve(dir, testConfig(), p)
	if err != nil {
		t.Fatal(err)
	}
	site := plan.Site
	site.Domains = append(site.Domains, "api.test")

	syncDomains(plan, site, p, "test")

	if got := readFile(t, filepath.Join(dir, ".lerd.yaml")); !strings.Contains(got, "api") {
		t.Errorf(".lerd.yaml = %q, want the api domain synced", got)
	}
}

// A name the user asked for is a choice the directory cannot reproduce, so it
// is recorded for the next machine that links the project.
func TestSyncDomains_recordsAnExplicitName(t *testing.T) {
	dir := projectDir(t, "demo", "php_version: \"8.4\"\n")
	p := CLIPolicy("shop", false, nil)
	plan, err := Resolve(dir, testConfig(), p)
	if err != nil {
		t.Fatal(err)
	}

	syncDomains(plan, plan.Site, p, "test")

	if got := readFile(t, filepath.Join(dir, ".lerd.yaml")); !strings.Contains(got, "shop") {
		t.Errorf(".lerd.yaml = %q, want the requested shop domain", got)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A hand-written local override is untracked by design, so the link keeps it
// out of the working tree status the way lerd's own writes of it already do.
func TestExcludeLocalOverride_excludesAnExistingLocalFile(t *testing.T) {
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, config.LocalOverrideFile), []byte("php_version: \"8.4\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	excludeLocalOverride(dir)
	excludeLocalOverride(dir)

	got := readFile(t, filepath.Join(dir, ".git", "info", "exclude"))
	if strings.Count(got, "/"+config.LocalOverrideFile) != 1 {
		t.Errorf("exclude = %q, want the local file listed once", got)
	}
}

func TestExcludeLocalOverride_leavesARepoWithoutALocalFileAlone(t *testing.T) {
	dir := gitRepo(t)

	excludeLocalOverride(dir)

	if got := readFile(t, filepath.Join(dir, ".git", "info", "exclude")); got != "" {
		t.Errorf("exclude = %q, want nothing written", got)
	}
}

func TestExcludeLocalOverride_ignoresADirectoryOutsideGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.LocalOverrideFile), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	excludeLocalOverride(dir)

	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Errorf("a .git directory was created outside a repository: %v", err)
	}
}
