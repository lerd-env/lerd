package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func questionsFor(t *testing.T, dir string) *ProjectQuestions {
	t.Helper()
	q, err := ProjectQuestionsFor(dir)
	if err != nil {
		t.Fatalf("ProjectQuestionsFor: %v", err)
	}
	return q
}

// A PHP project is asked the PHP questions: version, HTTPS, a database and the
// other services, the same set the terminal wizard puts on screen.
func TestProjectQuestionsForPHPProject(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"composer.json": `{"require":{"php":"^8.3"}}`})

	q := questionsFor(t, dir)
	if q.Kind != ProjectKindPHP {
		t.Fatalf("kind = %q, want %q", q.Kind, ProjectKindPHP)
	}
	if q.PHPVersion == "" {
		t.Error("no PHP version offered as the default")
	}
	if len(q.DatabaseOptions) == 0 {
		t.Error("no database options offered")
	}
	if len(q.ServiceOptions) == 0 {
		t.Error("no service options offered")
	}
}

// A directory with neither composer.json nor a framework is not a PHP project,
// so it gets the choice the terminal wizard offers before anything else.
func TestProjectQuestionsForBareDirectoryOffersAKindChoice(t *testing.T) {
	isolateSetupPlan(t)

	q := questionsFor(t, t.TempDir())
	if !q.KindChoice {
		t.Fatal("a directory lerd cannot classify should be asked how to run it")
	}
	if len(q.KindOptions) < 2 {
		t.Errorf("kind options = %v, want at least the proxy and container choices", q.KindOptions)
	}
}

// A Node project is recognised, so the plain-PHP answer is dropped and the dev
// server is the one to start from, with the project's own script offered.
func TestProjectQuestionsForNodeProject(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{
		"package.json": `{"scripts":{"dev":"vite"}}`,
	})

	q := questionsFor(t, dir)
	if q.Kind != ProjectKindProxy {
		t.Errorf("kind = %q, want %q", q.Kind, ProjectKindProxy)
	}
	for _, opt := range q.KindOptions {
		if opt.Value == ProjectKindPHP {
			t.Error("a recognised Node project should not be offered the plain PHP answer")
		}
	}
	if q.ProxyCommand == "" {
		t.Error("no dev command offered")
	}
	if q.ProxyPort == 0 {
		t.Error("no port offered")
	}
}

// A saved .lerd.yaml is what the answers start from, so re-running the
// questions from the dashboard shows what the project already committed to.
func TestProjectQuestionsSeedFromSavedConfig(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{
		"composer.json": `{"require":{}}`,
		".lerd.yaml":    "php_version: \"8.2\"\nservices:\n  - redis\n",
	})

	q := questionsFor(t, dir)
	if q.PHPVersion != "8.2" {
		t.Errorf("php version = %q, want the saved 8.2", q.PHPVersion)
	}
	found := false
	for _, s := range q.Services {
		if s == "redis" {
			found = true
		}
	}
	if !found {
		t.Errorf("services = %v, want the saved redis pre-selected", q.Services)
	}
}

// Answers become .lerd.yaml, which is what link then applies: the same file the
// terminal wizard writes, so a project configured from the dashboard is
// portable and re-linkable exactly like one configured on a terminal.
func TestSaveProjectAnswersWritesPHPConfig(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"composer.json": `{"require":{}}`})

	err := SaveProjectAnswers(dir, ProjectAnswers{
		Kind:       ProjectKindPHP,
		PHPVersion: "8.3",
		Secured:    true,
		Database:   "mysql",
		Services:   []string{"redis"},
		Workers:    []string{"queue"},
	})
	if err != nil {
		t.Fatalf("SaveProjectAnswers: %v", err)
	}

	saved, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.PHPVersion != "8.3" {
		t.Errorf("php_version = %q, want 8.3", saved.PHPVersion)
	}
	names := saved.ServiceNames()
	if len(names) != 2 || names[0] != "mysql" || names[1] != "redis" {
		t.Errorf("services = %v, want mysql then redis", names)
	}
	if len(saved.Workers) != 1 || saved.Workers[0] != "queue" {
		t.Errorf("workers = %v, want queue", saved.Workers)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); err != nil {
		t.Errorf(".lerd.yaml was not written: %v", err)
	}
}

// SQLite is an answer, not a service: it is what the project's own config says
// rather than something lerd installs, so it never lands in the services list.
func TestSaveProjectAnswersKeepsSQLiteOutOfServices(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"composer.json": `{"require":{}}`})

	if err := SaveProjectAnswers(dir, ProjectAnswers{Kind: ProjectKindPHP, Database: "sqlite"}); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.ServiceNames()) != 0 {
		t.Errorf("services = %v, want none", saved.ServiceNames())
	}
}

// The dev-server answers write the proxy section link reads, port included.
// Ticking a service asks for it, so link must not skip it as removed and leave
// .env wired to a container that never comes up. Others stay removed.
func TestSaveProjectAnswersBringsBackAPickedRemovedService(t *testing.T) {
	isolateSetupPlan(t)
	for _, name := range []string{"mailpit", "redis"} {
		if err := config.SetServiceRemoved(name, true); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"composer.json": `{"require":{}}`})

	if err := SaveProjectAnswers(dir, ProjectAnswers{Kind: ProjectKindPHP, Services: []string{"mailpit"}}); err != nil {
		t.Fatal(err)
	}

	if config.ServiceIsRemoved("mailpit") {
		t.Error("mailpit was picked but is still marked removed")
	}
	if !config.ServiceIsRemoved("redis") {
		t.Error("redis was not picked and should stay removed")
	}
}

func TestSaveProjectAnswersWritesProxyConfig(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()
	writePlanFixture(t, dir, map[string]string{"package.json": `{"scripts":{"dev":"vite"}}`})

	err := SaveProjectAnswers(dir, ProjectAnswers{
		Kind:         ProjectKindProxy,
		ProxyCommand: "npm run dev",
		ProxyPort:    5173,
		Services:     []string{"redis"},
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Proxy == nil {
		t.Fatal("no proxy section written")
	}
	if saved.Proxy.Command != "npm run dev" || saved.Proxy.Port != 5173 {
		t.Errorf("proxy = %+v, want the answered command and port", saved.Proxy)
	}
}

// A container project needs a port to proxy to; an answer without one would
// write a section link cannot serve.
func TestSaveProjectAnswersRejectsContainerWithoutPort(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()

	err := SaveProjectAnswers(dir, ProjectAnswers{Kind: ProjectKindContainer})
	if err == nil {
		t.Fatal("a container answer with no port should be refused")
	}
}

// A dev server has to bind a port too, and lerd cannot guess one after the fact.
func TestSaveProjectAnswersRejectsProxyWithoutPort(t *testing.T) {
	isolateSetupPlan(t)
	dir := t.TempDir()

	err := SaveProjectAnswers(dir, ProjectAnswers{Kind: ProjectKindProxy, ProxyCommand: "npm run dev"})
	if err == nil {
		t.Fatal("a proxy answer with no port should be refused")
	}
}

// A pinned Node version is the project's, not the wizard's to drop. Where lerd
// does not manage Node the question is never filled in, so the empty answer that
// comes back must not erase what .lerd.yaml already pins.
func TestProjectConfigFromAnswersKeepsAPinnedNodeVersion(t *testing.T) {
	setNodeManaged(t, false)
	defaults := &config.ProjectConfig{NodeVersion: "20"}

	for _, a := range []ProjectAnswers{
		{Kind: ProjectKindPHP, PHPVersion: "8.3"},
		{Kind: ProjectKindProxy, ProxyCommand: "npm run dev", ProxyPort: 5173},
		{Kind: ProjectKindContainer, ContainerPort: 8080},
	} {
		cfg, err := projectConfigFromAnswers(t.TempDir(), defaults, a, true)
		if err != nil {
			t.Fatalf("%s: %v", a.Kind, err)
		}
		if cfg.NodeVersion != "20" {
			t.Errorf("%s: node_version = %q, want the pin kept", a.Kind, cfg.NodeVersion)
		}
	}
}

// setNodeManaged persists the Node-management choice the wizard reads, so a test
// can put itself on either side of what an empty answer means.
func setNodeManaged(t *testing.T, managed bool) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetNodeManaged(managed)
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
}

// Where lerd manages Node the wizard offers an unpinned entry whose value is the
// empty string, so an empty answer is the user clearing the pin and has to be
// saved as one. Restoring the old value would leave no way to unpin from the
// dashboard at all.
func TestProjectConfigFromAnswersHonoursClearingTheNodePin(t *testing.T) {
	setNodeManaged(t, true)
	defaults := &config.ProjectConfig{NodeVersion: "20"}

	cfg, err := projectConfigFromAnswers(t.TempDir(), defaults, ProjectAnswers{
		Kind: ProjectKindPHP, PHPVersion: "8.3", NodeVersion: "",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NodeVersion != "" {
		t.Errorf("node_version = %q, want the pin cleared", cfg.NodeVersion)
	}
}

// The dashboard renders the Node question only for the PHP kind, so a proxy or
// container answer arrives empty even on a machine where lerd manages Node.
// That empty is nobody having been asked, and the pin stays.
func TestProjectConfigFromAnswersKeepsThePinWhereTheQuestionIsNotAsked(t *testing.T) {
	setNodeManaged(t, true)
	defaults := &config.ProjectConfig{NodeVersion: "22"}

	for _, a := range []ProjectAnswers{
		{Kind: ProjectKindProxy, ProxyCommand: "npm run dev", ProxyPort: 5173},
		{Kind: ProjectKindContainer, ContainerPort: 8080},
	} {
		cfg, err := projectConfigFromAnswers(t.TempDir(), defaults, a, true)
		if err != nil {
			t.Fatalf("%s: %v", a.Kind, err)
		}
		if cfg.NodeVersion != "22" {
			t.Errorf("%s: node_version = %q, want the pin kept", a.Kind, cfg.NodeVersion)
		}
	}
}
