package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func isolateWizardEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func phpProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func getJSON(t *testing.T, handler http.HandlerFunc, url string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.RemoteAddr = "127.0.0.1:5000"
	rr := httptest.NewRecorder()
	handler(rr, req)

	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding %s: %v (body %q)", url, err, rr.Body.String())
	}
	return out
}

// The create step reads the catalogue from the store, so the dashboard needs a
// frameworks endpoint at all — it never had one.
func TestHandleFrameworkCatalogue(t *testing.T) {
	isolateWizardEnv(t)

	out := getJSON(t, handleFrameworkCatalogue, "/api/frameworks/catalogue")
	frameworks, ok := out["frameworks"].([]any)
	if !ok {
		t.Fatalf("no frameworks in the response: %v", out)
	}
	if len(frameworks) == 0 {
		t.Fatal("the catalogue is empty; the bundled definitions should be offered")
	}
	first, _ := frameworks[0].(map[string]any)
	if first["name"] == "" || first["label"] == "" {
		t.Errorf("catalogue entry is missing its name or label: %v", first)
	}
}

// The questions step asks what the terminal wizard asks about the directory.
func TestHandleProjectQuestionsForPHPProject(t *testing.T) {
	isolateWizardEnv(t)
	dir := phpProjectDir(t)

	out := getJSON(t, handleProjectQuestions, "/api/project/questions?dir="+dir)
	if out["kind"] != "php" {
		t.Errorf("kind = %v, want php", out["kind"])
	}
	if out["database_options"] == nil {
		t.Error("no database options offered")
	}
}

func TestHandleProjectQuestionsRejectsUnknownDir(t *testing.T) {
	isolateWizardEnv(t)

	out := getJSON(t, handleProjectQuestions, "/api/project/questions?dir=/nope/does/not/exist")
	if out["error"] == nil {
		t.Error("a directory that does not exist should be an error")
	}
}

// Answering writes .lerd.yaml, the file the link that follows applies.
func TestHandleProjectQuestionsSavesAnswers(t *testing.T) {
	isolateWizardEnv(t)
	dir := phpProjectDir(t)

	body := `{"kind":"php","php_version":"8.3","database":"mysql","services":["redis"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/project/questions?dir="+dir, bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:5000"
	rr := httptest.NewRecorder()
	handleProjectQuestions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(dir, ".lerd.yaml"))
	if err != nil {
		t.Fatalf(".lerd.yaml was not written: %v", err)
	}
	if !bytes.Contains(saved, []byte("8.3")) || !bytes.Contains(saved, []byte("mysql")) {
		t.Errorf("saved config is missing the answers: %s", saved)
	}
}

// Writing into a project on the host is a host action, so a remote session
// without the opt-in is turned away.
func TestHandleProjectQuestionsPostRequiresHostAuthority(t *testing.T) {
	isolateWizardEnv(t)
	dir := phpProjectDir(t)

	req := httptest.NewRequest(http.MethodPost, "/api/project/questions?dir="+dir, bytes.NewBufferString(`{"kind":"php"}`))
	req.RemoteAddr = "8.8.8.8:5000"
	rr := httptest.NewRecorder()
	handleProjectQuestions(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lerd.yaml")); err == nil {
		t.Error("a refused request should not have written anything")
	}
}

// The setup step is a list before it is a run: the dashboard shows the same
// steps the terminal selector shows, with the same defaults ticked.
func TestHandleProjectSetupSteps(t *testing.T) {
	isolateWizardEnv(t)
	dir := phpProjectDir(t)

	out := getJSON(t, handleProjectSetupSteps, "/api/project/setup-steps?dir="+dir)
	steps, ok := out["steps"].([]any)
	if !ok || len(steps) == 0 {
		t.Fatalf("no steps offered: %v", out)
	}
	first, _ := steps[0].(map[string]any)
	if first["label"] != "composer install" {
		t.Errorf("first step = %v, want composer install", first["label"])
	}
	// The wizard ends on the site in the browser, so nothing in the list opens
	// one on the host.
	for _, step := range steps {
		s, _ := step.(map[string]any)
		if s["label"] == "lerd open" {
			t.Error("the dashboard plan should not offer to open a browser on the host")
		}
	}
}

// The wizard names a project folder by its absolute path, so anything else,
// a relative path or one that climbs, is refused before a file is read.
func TestRequestDir(t *testing.T) {
	base := t.TempDir()
	inside := filepath.Join(base, "app")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "a..b"), 0o755); err != nil {
		t.Fatal(err)
	}
	// From inside, both relative spellings name a folder that exists.
	t.Chdir(inside)
	cases := map[string]bool{
		inside:                         true,
		inside + "/":                   true,
		"":                             false,
		".":                            false,
		"../app":                       false,
		"..":                           false,
		inside + "/../app":             true, // cleaned to inside
		filepath.Join(base, "a..b"):    true,
		filepath.Join(base, "missing"): false,
	}
	for dir, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/project/setup-steps?dir="+url.QueryEscape(dir), nil)
		if _, got := requestDir(r); got != want {
			t.Errorf("requestDir(%q) = %v, want %v", dir, got, want)
		}
	}
}
