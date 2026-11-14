package ui

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/siteops"
)

// pluginInfo is what the dashboard needs to draw a plugin's rail page and site tab.
type pluginInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Page        *pluginPage `json:"page,omitempty"`
	SiteTab     *pluginPage `json:"site_tab,omitempty"`
	// OverviewCard is listed for every site; a site's switches say where it shows.
	OverviewCard *pluginPage `json:"overview_card,omitempty"`
	// Update says the catalog approved a newer commit, for the rail's badge.
	Update bool `json:"update,omitempty"`
}

type pluginPage struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Icon  string `json:"icon,omitempty"`
}

// handlePluginRevocations serves GET /api/plugin-revocations: the installed
// plugins the catalog revoked and that stay stopped until the user answers.
func handlePluginRevocations(w http.ResponseWriter, _ *http.Request) {
	list := plugins.Revoked()
	if list == nil {
		list = []plugins.Revocation{}
	}
	writeJSON(w, list)
}

// handlePluginsList serves GET /api/plugins.
func handlePluginsList(w http.ResponseWriter, _ *http.Request) {
	list, _ := plugins.Load()
	out := make([]pluginInfo, 0, len(list))
	for _, p := range list {
		info := pluginInfo{Name: p.Name, Description: p.Description}
		_, info.Update = plugins.CachedUpdate(p.Name)
		if p.UI != nil {
			info.Page = pluginPageInfo(p.Name, p.UI.Page)
			info.SiteTab = pluginPageInfo(p.Name, p.UI.SiteTab)
			info.OverviewCard = pluginPageInfo(p.Name, p.UI.OverviewCard)
		}
		out = append(out, info)
	}
	writeJSON(w, out)
}

func pluginPageInfo(name string, page *plugins.UIPage) *pluginPage {
	if page == nil {
		return nil
	}
	out := &pluginPage{Title: page.Title, URL: pluginFileURL(name, page.Entry)}
	if page.Icon != "" {
		out.Icon = pluginFileURL(name, page.Icon)
	}
	return out
}

func pluginFileURL(name, rel string) string { return "/plugins/" + name + "/" + rel }

// pluginSandboxCSP keeps a plugin page off lerd's origin even when it is opened
// outside the dashboard's frame, so its scripts can never call lerd's API with
// the dashboard's standing. It reaches its own endpoints through the frame bridge.
const pluginSandboxCSP = "sandbox allow-scripts allow-forms allow-popups allow-modals"

// pluginKit is lerd's look for plugin pages, served under /plugins/_lerd/; no
// plugin can take that name, since a name starts with a letter.
//
//go:embed pluginkit
var pluginKit embed.FS

// pluginKitTags goes first in every plugin page's head, so the page's own
// styles come after it.
const pluginKitTags = `<link rel="stylesheet" href="/plugins/_lerd/base.css"><script src="/plugins/_lerd/theme.js"></script>`

var headTagRe = regexp.MustCompile(`(?i)<head(\s[^>]*)?>`)

// injectPluginKit adds the kit right after the page's <head>, or ahead of
// everything when it has none.
func injectPluginKit(page []byte) []byte {
	loc := headTagRe.FindIndex(page)
	if loc == nil {
		return append([]byte(pluginKitTags), page...)
	}
	return slices.Concat(page[:loc[1]], []byte(pluginKitTags), page[loc[1]:])
}

// handlePluginUI serves GET /plugins/<name>/<file>: the plugin's ui/ folder,
// its icon and the screenshots its manifest lists, nothing else of it.
func handlePluginUI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(path.Clean(r.URL.Path), "/plugins/")
	name, rel, _ := strings.Cut(rest, "/")
	if name == "_lerd" {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFileFS(w, r, pluginKit, "pluginkit/"+rel)
		return
	}
	p, err := plugins.Find(name)
	if err != nil || !p.Serves(rel) {
		http.NotFound(w, r)
		return
	}
	// Serves checks the text; a link can still name any file the user can
	// read, so the file it resolves to must stay where the text said.
	within := filepath.Join(p.Dir, plugins.UIDir)
	if !strings.HasPrefix(rel, plugins.UIDir+"/") {
		within = p.Dir
	}
	file, ok := resolveWithin(within, filepath.Join(p.Dir, filepath.FromSlash(rel)))
	if !ok {
		http.NotFound(w, r)
		return
	}
	// ServeContent rather than ServeFile, which redirects index.html to its dir.
	f, err := os.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", pluginSandboxCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if ext := filepath.Ext(file); strings.EqualFold(ext, ".html") || strings.EqualFold(ext, ".htm") {
		page, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), bytes.NewReader(injectPluginKit(page)))
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// resolveWithin follows every link in path and reports the real file, only
// when it is inside dir once dir's own links are followed too.
func resolveWithin(dir, path string) (string, bool) {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(realDir, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return real, true
}

// handlePluginAPI serves /api/plugins/<name>/<path> by running the endpoint's
// file: the body on stdin, the method, query and site in LERD_* variables. A
// zero exit answers 200 with stdout, anything else 500 with what it printed.
func handlePluginAPI(w http.ResponseWriter, r *http.Request) {
	p, e := pluginEndpointFor(r.URL.Path)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	site, _ := config.FindSite(r.URL.Query().Get("site"))
	runPluginEndpoint(w, r, p, e, site, "")
}

// runPluginEndpoint answers one call to a plugin endpoint, for the site it
// concerns when there is one.
func runPluginEndpoint(w http.ResponseWriter, r *http.Request, p *plugins.Plugin, e *plugins.Endpoint, site *config.Site, branch string) {
	if !slices.Contains(endpointMethods(e), r.Method) {
		w.Header().Set("Allow", strings.Join(endpointMethods(e), ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := map[string]string{"LERD_METHOD": r.Method, "LERD_QUERY": r.URL.RawQuery}
	dir := p.Dir
	if site != nil {
		payload := plugins.SitePayload(site)
		payload["branch"] = branch
		for k, v := range plugins.Env(payload) {
			env[k] = v
		}
		if dir = resolveSitePath(site, branch); dir == "" {
			http.Error(w, "unknown worktree branch: "+branch, http.StatusNotFound)
			return
		}
	}
	cmd, err := p.Command(e.Run, plugins.SurfaceAPI, nil, env)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var stdout, stderr bytes.Buffer
	cmd.Dir = dir
	cmd.Stdin = io.LimitReader(r.Body, 1<<20)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := plugins.RunWithin(cmd, e.Limit()); err != nil {
		msg := strings.TrimSpace(stderr.String() + "\n" + err.Error())
		plugins.Failed(plugins.Failure{Plugin: p.Name, Surface: plugins.SurfaceAPI, What: e.Path, Error: msg})
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if json.Valid(stdout.Bytes()) {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	_, _ = w.Write(stdout.Bytes())
}

// endpointMethods defaults an endpoint that names none to GET, the one method
// that can't change anything by convention.
func endpointMethods(e *plugins.Endpoint) []string {
	if len(e.Methods) == 0 {
		return []string{http.MethodGet}
	}
	return e.Methods
}

// pluginEndpointFor resolves /api/plugins/<name>/<path> to its plugin endpoint.
func pluginEndpointFor(urlPath string) (*plugins.Plugin, *plugins.Endpoint) {
	rest, ok := strings.CutPrefix(urlPath, "/api/plugins/")
	if !ok {
		return nil, nil
	}
	name, sub, _ := strings.Cut(rest, "/")
	p, err := plugins.Find(name)
	if err != nil {
		return nil, nil
	}
	for i := range p.API {
		if p.API[i].Path == sub {
			return p, &p.API[i]
		}
	}
	return nil, nil
}

// withPluginPages serves <PagePath><name>/page.js and its /api/<path> calls,
// ahead of the remote gate, only through nginx and for sites that switched it
// on; loopback_only endpoints stay off pages, which a phone may be showing.
func withPluginPages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, plugins.PagePath)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if !fromNginx(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		name, sub, _ := strings.Cut(rest, "/")
		site, err := config.FindSite(r.Header.Get("X-Lerd-Site"))
		if err != nil || !slices.Contains(site.PagePlugins, name) {
			http.NotFound(w, r)
			return
		}
		p, err := plugins.Find(name)
		if err != nil || p.UI == nil || p.UI.PageScript == "" {
			http.NotFound(w, r)
			return
		}
		if sub == "page.js" {
			file, ok := resolveWithin(filepath.Join(p.Dir, plugins.UIDir), filepath.Join(p.Dir, filepath.FromSlash(p.UI.PageScript)))
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, file)
			return
		}
		endpoint, ok := strings.CutPrefix(sub, "api/")
		var e *plugins.Endpoint
		for i := range p.API {
			if ok && p.API[i].Path == endpoint {
				e = &p.API[i]
			}
		}
		switch {
		case e == nil:
			http.NotFound(w, r)
		case e.LoopbackOnly || !fromSitePage(r):
			http.Error(w, "Forbidden", http.StatusForbidden)
		default:
			runPluginEndpoint(w, r, p, e, site, r.Header.Get("X-Lerd-Branch"))
		}
	})
}

// pagePlugin is one thing a plugin can put on a site, its page script, its
// Overview card or its tab, and whether the site has it switched on.
type pagePlugin struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"`
	On          bool   `json:"on"`
}

const (
	kindPageScript   = "page_script"
	kindOverviewCard = "overview_card"
	kindSiteTab      = "site_tab"
)

// handlePagePlugins lists what plugins can put on ?site= (GET), or switches one
// on or off for a site (POST {site, plugin, kind, on}); kind defaults to the
// page script, the only kind before Overview cards.
func handlePagePlugins(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Site   string `json:"site"`
		Plugin string `json:"plugin"`
		Kind   string `json:"kind"`
		On     bool   `json:"on"`
	}
	switch r.Method {
	case http.MethodGet:
		req.Site = r.URL.Query().Get("site")
	case http.MethodPost:
		if !hasHostActionAuthority(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	site, err := config.FindSite(resolveSiteName(req.Site))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		set := siteops.SetPagePlugin
		switch req.Kind {
		case "", kindPageScript:
		case kindOverviewCard:
			set = siteops.SetOverviewCard
		case kindSiteTab:
			set = siteops.SetSiteTab
		default:
			http.Error(w, "unknown kind: "+req.Kind, http.StatusBadRequest)
			return
		}
		if err := set(site, req.Plugin, req.On); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if site, err = config.FindSite(site.Name); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	list, _ := plugins.Load()
	out := []pagePlugin{}
	for _, p := range list {
		if p.UI == nil {
			continue
		}
		if p.UI.PageScript != "" {
			out = append(out, pagePlugin{Name: p.Name, Description: p.Description, Kind: kindPageScript, On: slices.Contains(site.PagePlugins, p.Name)})
		}
		if p.UI.OverviewCard != nil {
			out = append(out, pagePlugin{Name: p.Name, Description: p.Description, Kind: kindOverviewCard, On: slices.Contains(site.OverviewCards, p.Name)})
		}
		if p.UI.SiteTab != nil {
			out = append(out, pagePlugin{Name: p.Name, Description: p.Description, Kind: kindSiteTab, On: !slices.Contains(site.HiddenTabs, p.Name)})
		}
	}
	writeJSON(w, out)
}

// managedPlugin is a row on the dashboard's plugin manager. A plugin that
// failed to load carries its error instead of a description.
type managedPlugin struct {
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Adds        []string `json:"adds,omitempty"`
	Requires    []string `json:"requires,omitempty"`
	Error       string   `json:"error,omitempty"`
	// Icon is drawn on its card: its own icon, or its rail page's.
	Icon string `json:"icon,omitempty"`
	// Author is who the catalog lists it by, for one installed from the catalog.
	Author string `json:"author,omitempty"`
	// Repo and Commit are where the catalog installed it from; empty when not listed.
	Repo   string `json:"repo,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Update is the newer approved commit, when there is one, with its version
	// and GitHub's comparison of the two commits.
	Update        string `json:"update,omitempty"`
	UpdateVersion string `json:"update_version,omitempty"`
	Changes       string `json:"changes,omitempty"`
	// Failure is the plugin's latest failure, with where to report it.
	Failure *pluginFailure `json:"failure,omitempty"`
	// Features names each thing it adds; Screenshots are their URLs; HasDocs
	// says GET /api/plugin-manager/<name>/docs has its instructions.
	Features    []plugins.Feature `json:"features,omitempty"`
	Screenshots []string          `json:"screenshots,omitempty"`
	HasDocs     bool              `json:"has_docs,omitempty"`
	// Revoked is the catalog's revocation stopping it, for its card to explain.
	Revoked *plugins.Revocation `json:"revoked,omitempty"`
	// Notifications is none, pending, allowed or denied; pending is the
	// dashboard's cue to ask.
	Notifications string `json:"notifications,omitempty"`
	// Schedule is where it stands on running on a schedule, the same four
	// states, and Schedules what it would run and how the last runs went.
	Schedule  string                   `json:"schedule,omitempty"`
	Schedules []plugins.ScheduleStatus `json:"schedules,omitempty"`
}

type pluginFailure struct {
	plugins.Failure
	ReportURL string `json:"report_url,omitempty"`
}

// failureFor is the plugin's latest failure as the dashboard draws it.
func failureFor(name string) *pluginFailure {
	f, ok := plugins.LastFailure(name)
	if !ok {
		return nil
	}
	return &pluginFailure{Failure: f, ReportURL: f.ReportURL()}
}

// catalogEntry is a catalog listing as the Browse view draws it.
type catalogEntry struct {
	plugins.Listing
	Installed  bool `json:"installed"`
	Compatible bool `json:"compatible"`
	// RunsHere is false when the plugin lists platforms and this is not one.
	RunsHere bool `json:"runs_here"`
	// Missing is what the plugin requires that this machine lacks.
	Missing []string `json:"missing,omitempty"`
}

// handlePluginManager serves /api/plugin-manager: GET lists installed plugins
// (or /catalog the approved ones), POST installs {name} or {source}, and
// POST <name>/update or DELETE <name> updates or removes, keeping its data.
func handlePluginManager(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/plugin-manager"), "/")
	switch {
	case r.Method == http.MethodGet && name == "":
		writeJSON(w, managedPlugins())
	case r.Method == http.MethodGet && name == "catalog":
		// Opening Browse asks for the catalog as it is now, not as of the hourly
		// check; within a minute of the last fetch, or offline, the copy held answers.
		if r.URL.Query().Get("fresh") != "" && plugins.RefreshCatalogOlderThan(time.Minute) == nil {
			broker.broadcastPlugins()
		}
		entries, err := catalogEntries()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, entries)
	case r.Method == http.MethodGet && strings.HasSuffix(name, "/logs"):
		// A plugin that no longer loads keeps its log, the one place saying why.
		plugin := strings.TrimSuffix(name, "/logs")
		if !plugins.ValidName(plugin) {
			http.NotFound(w, r)
			return
		}
		out := []pluginFailure{}
		for _, f := range plugins.Failures(plugin) {
			out = append(out, pluginFailure{Failure: f, ReportURL: f.ReportURL()})
		}
		writeJSON(w, out)
	case r.Method == http.MethodGet && strings.HasSuffix(name, "/docs"):
		p, err := plugins.Find(strings.TrimSuffix(name, "/docs"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		html, err := p.DocsHTML(func(rel string) string { return pluginFileURL(p.Name, rel) })
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"html": html})
	case r.Method == http.MethodPost && strings.HasSuffix(name, "/keep"):
		// Running a revoked plugin again is the user's call, never the plugin's.
		if r.Header.Get(plugins.TokenHeader) != "" {
			http.Error(w, "a plugin cannot answer for the user", http.StatusForbidden)
			return
		}
		if err := plugins.KeepRevoked(strings.TrimSuffix(name, "/keep")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case r.Method == http.MethodPost && permissionOf(name) != "":
		// The answer is the user's: a plugin's own token never gives it.
		if r.Header.Get(plugins.TokenHeader) != "" {
			http.Error(w, "a plugin cannot answer for the user", http.StatusForbidden)
			return
		}
		var req struct {
			Allowed bool `json:"allowed"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		perm := permissionOf(name)
		if err := plugins.SetPermission(strings.TrimSuffix(name, "/"+perm), perm, req.Allowed); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case r.Method == http.MethodPost && strings.HasSuffix(name, "/update"):
		if err := plugins.UpdateListed(strings.TrimSuffix(name, "/update")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	case r.Method == http.MethodPost && name == "":
		var req struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var p *plugins.Plugin
		var err error
		switch {
		case req.Name != "":
			p, err = plugins.InstallListed(req.Name)
		case strings.TrimSpace(req.Source) != "":
			p, err = plugins.Install(strings.TrimSpace(req.Source))
		default:
			http.Error(w, "name or source is required", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, managedPlugin{Name: p.Name, Version: p.Version, Description: p.Description, Adds: p.Surfaces(), Requires: p.Requirements()})
	case r.Method == http.MethodDelete && name != "":
		if err := serviceops.RemovePlugin(name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// permissionOf is the permission a /api/plugin-manager/<name>/<permission>
// path answers, or empty for any other path.
func permissionOf(path string) string {
	for _, perm := range plugins.Permissions {
		if strings.HasSuffix(path, "/"+perm) {
			return perm
		}
	}
	return ""
}

func managedPlugins() []managedPlugin {
	list, errs := plugins.Load()
	out := make([]managedPlugin, 0, len(list)+len(errs))
	for _, p := range list {
		row := managedPlugin{Name: p.Name, Version: p.Version, Description: p.Description, Adds: p.Surfaces(), Requires: p.Requirements(), Failure: failureFor(p.Name),
			Features: p.Features(), HasDocs: p.Docs != "", Notifications: plugins.NotificationState(p),
			Schedule: plugins.PermissionState(p, plugins.PermissionSchedule)}
		if len(p.Schedules) > 0 {
			row.Schedules = plugins.ScheduleStatuses(p)
		}
		for _, rel := range p.Screenshots {
			row.Screenshots = append(row.Screenshots, pluginFileURL(p.Name, rel))
		}
		if icon := p.CardIcon(); icon != "" {
			row.Icon = pluginFileURL(p.Name, icon)
		}
		if src, ok := plugins.InstalledSource(p.Name); ok {
			row.Repo, row.Commit = src.Repo, src.Commit
			if l, ok := plugins.CachedListing(p.Name); ok && l.Repo == src.Repo {
				row.Author = l.Author
			}
			if l, newer := plugins.CachedUpdate(p.Name); newer {
				row.Update, row.UpdateVersion, row.Changes = l.Commit, l.Version, l.ChangesURL(src.Commit)
			}
		}
		out = append(out, row)
	}
	for _, e := range errs {
		row := managedPlugin{Name: filepath.Base(e.Dir), Error: e.Error()}
		if rev := (plugins.RevokedError{}); errors.As(e.Err, &rev) {
			row.Revoked = &rev.Revocation
			// The fixed commit is the way out of a revocation, so offer it
			// without loading the revoked code.
			if src, ok := plugins.InstalledSource(row.Name); ok {
				row.Repo, row.Commit = src.Repo, src.Commit
				if l, newer := plugins.CachedUpdate(row.Name); newer {
					row.Update, row.UpdateVersion, row.Changes = l.Commit, l.Version, l.ChangesURL(src.Commit)
				}
			}
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b managedPlugin) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func catalogEntries() ([]catalogEntry, error) {
	c, err := plugins.LoadCatalog()
	if err != nil {
		return nil, err
	}
	out := make([]catalogEntry, 0, len(c.Plugins))
	for _, l := range c.Plugins {
		_, err := os.Stat(filepath.Join(plugins.Dir(), l.Name, plugins.ManifestFile))
		out = append(out, catalogEntry{Listing: l, Installed: err == nil, Compatible: plugins.Compatible(l.Lerd), RunsHere: plugins.RunsHere(l.Platforms), Missing: plugins.Missing(l.Requires)})
	}
	return out, nil
}

// isLoopbackOnlyPluginPath reports whether path is a plugin endpoint its
// plugin declared loopback_only.
func isLoopbackOnlyPluginPath(path string) bool {
	_, e := pluginEndpointFor(path)
	return e != nil && e.LoopbackOnly
}

// hasValidPluginToken reports whether the request carries an installed
// plugin's API token, which a script sends in place of a browser's CSRF proof.
func hasValidPluginToken(r *http.Request) bool {
	return plugins.PluginForToken(r.Header.Get(plugins.TokenHeader)) != ""
}
