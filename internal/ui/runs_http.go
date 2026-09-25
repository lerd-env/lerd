package ui

import (
	"encoding/json"
	"fmt"
	"github.com/geodro/lerd/internal/siteops"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// RunRequest is the JSON body for POST /api/runs. The dashboard names a kind of
// work rather than a command line: every argv lerd-ui will run is built here,
// so nothing a page sends decides what executes on the host.
type RunRequest struct {
	Kind             string   `json:"kind"`
	Dir              string   `json:"dir"`
	Name             string   `json:"name,omitempty"`
	Framework        string   `json:"framework,omitempty"`
	FrameworkVersion string   `json:"framework_version,omitempty"`
	Steps            []string `json:"steps,omitempty"`
}

// The kinds of run the dashboard can start.
const (
	runKindScaffold = "scaffold"
	runKindLink     = "link"
	runKindEnv      = "env"
	runKindSetup    = "setup"
)

// runProjectName reports whether name is a single directory name lerd can
// create under a chosen parent. The browser picks the parent, so the name is
// the only free text in a scaffold, and it may not climb out of it.
func runProjectName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, "-") {
		return false
	}
	return name == filepath.Clean(name) && siteops.CheckProjectName(name) == nil
}

// runArgv turns a request into the command lerd-ui runs, the directory to run
// it in, and a label for the dashboard. An unknown kind is an error rather than
// anything executable.
func runArgv(self string, req RunRequest) (argv []string, dir, label string, err error) {
	dir = filepath.Clean(req.Dir)
	info, statErr := os.Stat(dir)
	if statErr != nil || !info.IsDir() {
		return nil, "", "", fmt.Errorf("not a valid directory: %s", req.Dir)
	}

	switch req.Kind {
	case runKindScaffold:
		if !runProjectName(req.Name) {
			return nil, "", "", fmt.Errorf("invalid project name: %q", req.Name)
		}
		target := filepath.Join(dir, strings.TrimSpace(req.Name))
		if _, err := os.Stat(target); err == nil {
			return nil, "", "", fmt.Errorf("%s already exists", target)
		}
		argv = []string{self, "new", target}
		if req.Framework != "" {
			argv = append(argv, "--framework="+req.Framework)
			if req.FrameworkVersion != "" {
				argv = append(argv, "--framework-version="+req.FrameworkVersion)
			}
		}
		return argv, dir, target, nil

	case runKindLink:
		// --yes: choosing Link in the wizard is the explicit consent the
		// host-proxy confirmation would otherwise ask for at the CLI.
		return []string{self, "link", "--yes"}, dir, "", nil

	case runKindEnv:
		return []string{self, "env"}, dir, "", nil

	case runKindSetup:
		if len(req.Steps) == 0 {
			return nil, "", "", fmt.Errorf("name at least one setup step to run")
		}
		argv = []string{self, "setup", "--skip-open"}
		for _, step := range req.Steps {
			if strings.TrimSpace(step) == "" {
				return nil, "", "", fmt.Errorf("empty setup step")
			}
			argv = append(argv, "--step", step)
		}
		return argv, dir, strings.Join(req.Steps, ", "), nil
	}
	return nil, "", "", fmt.Errorf("unknown run kind: %q", req.Kind)
}

// handleRuns serves POST /api/runs (start one) and GET /api/runs?dir= (what is
// already going, so a reloaded page reattaches instead of starting again).
func handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// No directory means every run: that is how a page asks whether anything
		// is still going before it draws a spinner on the button that resumes it.
		dir := r.URL.Query().Get("dir")
		if dir != "" {
			dir = filepath.Clean(dir)
		}
		writeJSON(w, map[string]any{"runs": runs.ForDir(dir)})
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Every kind of run reaches the host: it creates directories, runs composer,
	// registers sites. A remote dashboard needs the same opt-in the editor and
	// the other host actions need.
	if !hasHostActionAuthority(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"error": "invalid request body"})
		return
	}

	self, err := os.Executable()
	if err != nil {
		writeJSON(w, map[string]any{"error": "resolving executable: " + err.Error()})
		return
	}

	argv, dir, label, err := runArgv(self, req)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}

	started := runs.Start(req.Kind, dir, label, argv)
	writeJSON(w, map[string]any{"run": started.snapshot()})
}

// handleRunStream serves GET /api/runs/{id}/stream: the run's output so far,
// then the rest of it as it arrives, then a done event carrying how it ended.
// Replaying from the buffer is what lets a reloaded page pick a run back up
// mid-composer without having missed anything.
func handleRunStream(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	id = strings.TrimSuffix(id, "/stream")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	current, ok := runs.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	from := 0
	for {
		wait := current.wait()
		lines, next, done := current.read(from)
		from = next
		for _, line := range lines {
			// SSE ends a field at a carriage return as readily as at a newline, so
			// the \r a progress bar writes mid-line would split one line of output
			// into two frames. Nothing else needs escaping: the client reads the
			// payload as it arrives.
			fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(line, "\r", ""))
		}
		flusher.Flush()
		if done {
			snap := current.snapshot()
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", mustJSON(map[string]any{
				"ok":    snap.Status == runDone,
				"error": snap.Error,
				"id":    snap.ID,
			}))
			flusher.Flush()
			return
		}
		select {
		case <-wait:
		case <-r.Context().Done():
			// The page went away. The run keeps going; only this view of it ends.
			return
		}
	}
}
