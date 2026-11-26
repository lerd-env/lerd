package ui

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	lerdcli "github.com/geodro/lerd/internal/cli"
)

// handleFrameworkCatalogue serves GET /api/frameworks/catalogue: what a new
// project can be scaffolded from, read from the store rather than from a list
// the dashboard carries of its own.
func handleFrameworkCatalogue(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"frameworks": lerdcli.FrameworkCatalogue()})
}

// requestDir resolves the ?dir= parameter to an existing directory, named by
// its absolute path. Cleaned from the root, such a path cannot climb above it,
// while a folder named a..b stays reachable.
func requestDir(r *http.Request) (string, bool) {
	raw := r.URL.Query().Get("dir")
	if !filepath.IsAbs(raw) {
		return "", false
	}
	dir := filepath.Clean("/" + raw)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
}

// handleProjectQuestions serves the configuration step of the site wizard: GET
// returns the questions `lerd init` asks about a directory, POST saves the
// answers to its .lerd.yaml, which the link that follows then applies.
func handleProjectQuestions(w http.ResponseWriter, r *http.Request) {
	dir, ok := requestDir(r)
	if !ok {
		writeJSON(w, map[string]any{"error": "not a valid directory"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		questions, err := lerdcli.ProjectQuestionsFor(dir)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, questions)

	case http.MethodPost:
		// Answering writes a file into the project on the host, so a remote
		// session needs the same opt-in every other host action needs.
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var answers lerdcli.ProjectAnswers
		if err := json.NewDecoder(r.Body).Decode(&answers); err != nil {
			writeJSON(w, map[string]any{"error": "invalid request body"})
			return
		}
		if err := lerdcli.SaveProjectAnswers(dir, answers); err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectSetupSteps serves GET /api/project/setup-steps?dir=: the steps
// `lerd setup` offers for a directory. Listing runs nothing, so it is safe to
// call on a project the user is only looking at; running a step is a run.
func handleProjectSetupSteps(w http.ResponseWriter, r *http.Request) {
	dir, ok := requestDir(r)
	if !ok {
		writeJSON(w, map[string]any{"error": "not a valid directory"})
		return
	}
	// skipOpen: the wizard ends on the site in the dashboard, and a browser the
	// host opens is the wrong ending for a session that may not be on it.
	steps := lerdcli.SetupStepPlanFor(dir, true)
	if steps == nil {
		steps = []lerdcli.SetupStepInfo{}
	}
	writeJSON(w, map[string]any{"steps": steps})
}
