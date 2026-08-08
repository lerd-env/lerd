package ui

import (
	"encoding/json"
	"net/http"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/sitedoctor"
)

// doctorRoute handles the doctor subroutes for a site. It requires
// dashboard-control authority because checks and fixes execute in containers.
// Returns true when it owns the request. The check logic itself lives in
// internal/sitedoctor so the TUI and CLI share it.
//
//	GET  /api/sites/{domain}/doctor                 → run checks
//	POST /api/sites/{domain}/doctor/fix/{key}/run   → run a package-manager fix
func doctorRoute(w http.ResponseWriter, r *http.Request, domain string, rest []string) bool {
	if len(rest) == 0 || rest[0] != "doctor" {
		return false
	}
	if !hasHostActionAuthority(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return true
	}
	site, err := config.FindSiteByDomain(domain)
	if err != nil {
		writeJSON(w, map[string]any{"error": "site not found: " + domain})
		return true
	}
	switch {
	case len(rest) == 1 && r.Method == http.MethodGet:
		handleDoctorRun(w, r, site)
	case len(rest) == 4 && rest[1] == "fix" && rest[3] == "run" && r.Method == http.MethodPost:
		handleDoctorFixRun(w, r, site, rest[2])
	default:
		http.NotFound(w, r)
	}
	return true
}

func handleDoctorRun(w http.ResponseWriter, r *http.Request, site *config.Site) {
	branch := r.URL.Query().Get("branch")
	path, ok := resolveDoctorPath(w, site, branch)
	if !ok {
		return
	}
	// Freshly added worktrees don't carry .env (it's gitignored), so materialise
	// it first — otherwise every file check reads a missing .env and reports a
	// healthy worktree as broken. No-op for the parent and idempotent.
	ensureWorktreeEnvIfBranch(site, branch)
	writeJSON(w, sitedoctor.RunForPath(r.Context(), path, site.Framework))
}

// handleDoctorFixRun runs an allowlisted package-manager fix (composer
// install/update, npm install, npm audit fix) and streams its output as SSE,
// reusing the command runner's stream and per-site run lock.
func handleDoctorFixRun(w http.ResponseWriter, r *http.Request, site *config.Site, key string) {
	// Regenerating a vhost writes on the host and reloads nginx, so it never
	// reaches the container shell the package-manager fixes stream through.
	if key == sitedoctor.FixVhostRegenerate {
		err := sitedoctor.FixVhost(site.Path)
		streamHostAction(w, "regenerated the vhost for "+site.PrimaryDomain()+" and reloaded nginx", err)
		return
	}
	// Installing a service is a host action too, and a streaming one: the
	// pull can take minutes, so each phase is reported as it happens rather
	// than the request sitting silent until it finishes.
	if key == sitedoctor.FixInstallServices {
		handleDoctorInstallServices(w, r, site)
		return
	}
	shell, ok := sitedoctor.DoctorFixCommands[key]
	if !ok {
		writeJSON(w, map[string]any{"error": "unknown doctor fix: " + key})
		return
	}
	branch := r.URL.Query().Get("branch")
	path, ok := resolveDoctorPath(w, site, branch)
	if !ok {
		return
	}
	release, busyWith, ok := tryAcquireRun(siteRunLockKey(site), key)
	if !ok {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": "another command is already running on this site: " + busyWith})
		return
	}
	defer release()
	streamShellRun(w, r.Context(), path, shell, false)
}

// handleDoctorInstallServices installs every service the site declares and the
// machine does not have, one after another, streaming each install's phases.
// It works the set out again rather than trusting the client, so the fix can
// only ever install what the check would have reported.
func handleDoctorInstallServices(w http.ResponseWriter, r *http.Request, site *config.Site) {
	branch := r.URL.Query().Get("branch")
	path, ok := resolveDoctorPath(w, site, branch)
	if !ok {
		return
	}
	fw, _ := config.GetFrameworkForDir(site.Framework, path)
	missing := sitedoctor.MissingDeclaredServices(path, fw)
	if len(missing) == 0 {
		streamHostAction(w, "every service this site declares is already installed", nil)
		return
	}

	release, busyWith, ok := tryAcquireRun(siteRunLockKey(site), sitedoctor.FixInstallServices)
	if !ok {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": "another command is already running on this site: " + busyWith})
		return
	}
	defer release()

	send, ok := sseSender(w)
	if !ok {
		return
	}
	var failed error
	for _, name := range missing {
		send("stdout", "installing "+name)
		_, err := serviceops.InstallPresetStreaming(name, "", func(ev serviceops.PhaseEvent) {
			if line := installPhaseLine(name, ev); line != "" {
				send("stdout", line)
			}
		})
		if err != nil {
			send("stderr", name+": "+err.Error())
			failed = err
			break
		}
		send("stdout", name+" is installed and running")
	}
	exit := 0
	if failed != nil {
		exit = 1
	}
	body, _ := json.Marshal(map[string]any{"exit": exit, "durationMs": 0})
	send("done", string(body))
}

// installPhaseLine renders one install phase as a line of output, skipping the
// pull's own progress chatter, which arrives many times a second and says
// nothing a doctor fix log needs.
func installPhaseLine(name string, ev serviceops.PhaseEvent) string {
	switch ev.Phase {
	case "pulling_image":
		if ev.Message != "" {
			return ""
		}
		return name + ": pulling " + ev.Image
	case "installing_config":
		return name + ": writing config"
	case "starting_deps":
		return name + ": dependency " + ev.Dep + " " + ev.State
	case "starting_unit":
		return name + ": starting " + ev.Unit
	case "waiting_ready":
		return name + ": waiting for it to be ready"
	}
	return ""
}

// resolveDoctorPath returns the project path for the site, refusing an
// unresolved worktree branch rather than falling back to the parent checkout
// (which would diagnose or mutate the main site's files and database).
func resolveDoctorPath(w http.ResponseWriter, site *config.Site, branch string) (string, bool) {
	if branch == "" {
		return site.Path, true
	}
	wt := resolveSitePath(site, branch)
	if wt == "" {
		writeJSON(w, map[string]any{"error": "unknown worktree branch: " + branch})
		return "", false
	}
	return wt, true
}
