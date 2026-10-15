package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
	"github.com/geodro/lerd/internal/siteops"
)

// pluginInfo is what the dashboard needs to draw a plugin's rail page and site tab.
type pluginInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Page        *pluginPage `json:"page,omitempty"`
	SiteTab     *pluginPage `json:"site_tab,omitempty"`
}

type pluginPage struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Icon  string `json:"icon,omitempty"`
}

// handlePluginsList serves GET /api/plugins.
func handlePluginsList(w http.ResponseWriter, _ *http.Request) {
	list, _ := plugins.Load()
	out := make([]pluginInfo, 0, len(list))
	for _, p := range list {
		info := pluginInfo{Name: p.Name, Description: p.Description}
		if p.UI != nil {
			info.Page = pluginPageInfo(p.Name, p.UI.Page)
			info.SiteTab = pluginPageInfo(p.Name, p.UI.SiteTab)
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

// handlePluginUI serves GET /plugins/<name>/ui/<file>, the one directory of a
// plugin the dashboard exposes.
func handlePluginUI(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(path.Clean(r.URL.Path), "/plugins/")
	name, rel, _ := strings.Cut(rest, "/")
	if !strings.HasPrefix(rel, plugins.UIDir+"/") {
		http.NotFound(w, r)
		return
	}
	p, err := plugins.Find(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// ServeContent rather than ServeFile, which redirects index.html to its dir.
	f, err := os.Open(filepath.Join(p.Dir, filepath.FromSlash(rel)))
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
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
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
		dir = site.Path
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

// withPluginPages serves what page plugins put on a site's pages, ahead of the
// remote-control gate like browser logs: GET <PagePath><name>/page.js is the
// script, and <PagePath><name>/api/<path> reaches the plugin's endpoints. Both
// come only through nginx, which names the site, and only for a site the user
// switched the plugin on for. A loopback_only endpoint is never reachable from
// a page, which a phone on the LAN may be showing.
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
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(p.Dir, filepath.FromSlash(p.UI.PageScript)))
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

// pagePlugin is one plugin a site's pages can carry, and whether they do.
type pagePlugin struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	On          bool   `json:"on"`
}

// handlePagePlugins lists the page plugins for ?site= (GET) or switches one on
// or off for a site (POST {site, plugin, on}).
func handlePagePlugins(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Site   string `json:"site"`
		Plugin string `json:"plugin"`
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
		if err := siteops.SetPagePlugin(site, req.Plugin, req.On); err != nil {
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
		if p.UI != nil && p.UI.PageScript != "" {
			out = append(out, pagePlugin{Name: p.Name, Description: p.Description, On: slices.Contains(site.PagePlugins, p.Name)})
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
	// Commit is the catalog commit it was installed at; empty when not listed.
	Commit string `json:"commit,omitempty"`
	// Update is the newer approved commit, when there is one.
	Update string `json:"update,omitempty"`
	// Failure is the plugin's latest failure, with where to report it.
	Failure *pluginFailure `json:"failure,omitempty"`
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
	// Missing is what the plugin requires that this machine lacks.
	Missing []string `json:"missing,omitempty"`
}

// handlePluginManager serves the plugin manager:
//
//	GET    /api/plugin-manager                → installed plugins, broken ones included
//	GET    /api/plugin-manager/catalog        → approved plugins, marked when installed
//	POST   /api/plugin-manager                → install {"name"} from the catalog, or {"source"}
//	POST   /api/plugin-manager/<name>/update  → move to the newly approved commit
//	DELETE /api/plugin-manager/<name>         → remove, keeping the plugin's data
func handlePluginManager(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/plugin-manager"), "/")
	switch {
	case r.Method == http.MethodGet && name == "":
		writeJSON(w, managedPlugins())
	case r.Method == http.MethodGet && name == "catalog":
		entries, err := catalogEntries()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, entries)
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
		if err := plugins.Remove(name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func managedPlugins() []managedPlugin {
	list, errs := plugins.Load()
	out := make([]managedPlugin, 0, len(list)+len(errs))
	for _, p := range list {
		row := managedPlugin{Name: p.Name, Version: p.Version, Description: p.Description, Adds: p.Surfaces(), Requires: p.Requirements(), Failure: failureFor(p.Name)}
		if src, ok := plugins.InstalledSource(p.Name); ok {
			row.Commit = src.Commit
			if l, newer := plugins.PendingUpdate(p.Name); newer {
				row.Update = l.Commit
			}
		}
		out = append(out, row)
	}
	for _, e := range errs {
		out = append(out, managedPlugin{Name: filepath.Base(e.Dir), Error: e.Error()})
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
		out = append(out, catalogEntry{Listing: l, Installed: err == nil, Compatible: plugins.Compatible(l.Lerd), Missing: plugins.Missing(l.Requires)})
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
