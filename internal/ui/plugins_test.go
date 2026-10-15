package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/plugins"
)

// installAPIPlugin installs "echo" with an open endpoint, a loopback-only one
// and a page, and isolates the data dir it lives in.
func installAPIPlugin(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join(plugins.Dir(), "echo")
	for _, d := range []string{"bin", "ui"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		plugins.ManifestFile: `api_version: 1
name: echo
api:
  - {path: say, methods: [GET, POST], run: bin/say}
  - {path: host, methods: [POST], run: bin/say, loopback_only: true}
ui:
  page: {title: Echo, icon: ui/icon.svg, entry: ui/index.html}
  site_tab: {title: Echo, entry: ui/site.html}
`,
		"bin/say":       "#!/bin/sh\nin=$(cat)\n[ \"$LERD_QUERY\" = \"fail=1\" ] && { echo nope >&2; exit 1; }\nprintf '{\"method\":\"%s\",\"query\":\"%s\",\"body\":\"%s\",\"surface\":\"%s\"}' \"$LERD_METHOD\" \"$LERD_QUERY\" \"$in\" \"$LERD_SURFACE\"\n",
		"ui/index.html": "<p>echo</p>",
		"ui/site.html":  "<p>site</p>",
		"ui/icon.svg":   "<svg/>",
	}
	for rel, body := range files {
		mode := os.FileMode(0o644)
		if strings.HasPrefix(rel, "bin/") {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
}

func localRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:50000"
	return req
}

func TestPluginEndpoint_RunsWithRequestOnStdinAndEnv(t *testing.T) {
	installAPIPlugin(t)
	rec := httptest.NewRecorder()
	handlePluginAPI(rec, localRequest(http.MethodPost, "/api/plugins/echo/say?x=1", "hi"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("JSON output should be served as JSON, got %q", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"method": "POST", "query": "x=1", "body": "hi", "surface": "api"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestPluginEndpoint_Errors(t *testing.T) {
	installAPIPlugin(t)
	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/plugins/echo/say?fail=1", http.StatusInternalServerError},
		{http.MethodDelete, "/api/plugins/echo/say", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/plugins/echo/missing", http.StatusNotFound},
		{http.MethodGet, "/api/plugins/nobody/say", http.StatusNotFound},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		handlePluginAPI(rec, localRequest(c.method, c.path, ""))
		if rec.Code != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.path, rec.Code, c.want)
		}
	}
	if f, ok := plugins.LastFailure("echo"); !ok || f.Surface != plugins.SurfaceAPI || f.What != "say" || !strings.Contains(f.Error, "nope") {
		t.Errorf("a failing endpoint is recorded, got %+v %v", f, ok)
	}
}

func TestPluginFailureNotification(t *testing.T) {
	n := pluginFailureNotification(plugins.Failure{Plugin: "echo", Surface: plugins.SurfaceHook, What: "site.linked", Error: "boom\nmore"})
	if n.Kind != "plugin_failed" || n.URL != "#system/plugins" || n.Params["plugin"] != "echo" || n.Params["message"] != "boom" {
		t.Errorf("notification %+v", n)
	}
}

// Plugin endpoints sit behind the same gate as lerd's own: open ones answer an
// authenticated remote dashboard, loopback_only ones need remote full access.
func TestPluginEndpoint_RemoteGate(t *testing.T) {
	for _, full := range []bool{false, true} {
		setupConfigDirFullAccess(t, "alice", "s3cret", full)
		installAPIPlugin(t)

		for path, wantOpen := range map[string]bool{"/api/plugins/echo/say": true, "/api/plugins/echo/host": full} {
			next := &nextHandler{}
			rec := httptest.NewRecorder()
			withRemoteControlGate(next).ServeHTTP(rec, remoteRequest(http.MethodPost, path))
			if next.called != wantOpen {
				t.Errorf("full=%v %s: reached handler = %v, want %v (status %d)", full, path, next.called, wantOpen, rec.Code)
			}
		}
	}
}

// A plugin script has no browser to prove it is not a cross-site request; its
// token is that proof, and a wrong token is no proof at all.
func TestPluginToken_ClearsCSRF(t *testing.T) {
	setupConfigDirFullAccess(t, "", "", false)
	installAPIPlugin(t)
	token, err := plugins.Token("echo")
	if err != nil {
		t.Fatal(err)
	}
	for header, wantOpen := range map[string]bool{"": false, "bogus": false, token: true} {
		req := localRequest(http.MethodPost, "/api/sites/x/secure", "")
		if header != "" {
			req.Header.Set(plugins.TokenHeader, header)
		}
		next := &nextHandler{}
		withRemoteControlGate(next).ServeHTTP(httptest.NewRecorder(), req)
		if next.called != wantOpen {
			t.Errorf("token %q: reached handler = %v, want %v", header, next.called, wantOpen)
		}
	}
}

func TestPluginUI_ServesOnlyUIDirSandboxed(t *testing.T) {
	installAPIPlugin(t)

	rec := httptest.NewRecorder()
	handlePluginUI(rec, localRequest(http.MethodGet, "/plugins/echo/ui/index.html", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "echo") {
		t.Fatalf("entry not served: %d %s", rec.Code, rec.Body)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || strings.Contains(csp, "allow-same-origin") {
		t.Errorf("plugin pages must be sandboxed off lerd's origin, CSP = %q", csp)
	}

	for _, path := range []string{"/plugins/echo/plugin.yaml", "/plugins/echo/bin/say", "/plugins/echo/ui/../plugin.yaml"} {
		rec := httptest.NewRecorder()
		handlePluginUI(rec, localRequest(http.MethodGet, path, ""))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, rec.Code)
		}
	}
}

// The dashboard at lerd.localhost loads plugin files from lerd-ui's own port,
// and a CSS mask is a CORS fetch: without the header the rail icon is blank.
func TestPluginUI_AllowsTheDashboardOrigin(t *testing.T) {
	installAPIPlugin(t)
	req := localRequest(http.MethodGet, "/plugins/echo/ui/icon.svg", "")
	req.Header.Set("Origin", "http://lerd.localhost")
	rec := httptest.NewRecorder()
	withCORS(handlePluginUI)(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://lerd.localhost" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestPluginManager_ListsInstalledAndBroken(t *testing.T) {
	installAPIPlugin(t)
	broken := filepath.Join(plugins.Dir(), "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, plugins.ManifestFile), []byte("api_version: 99\nname: broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager", ""))
	var got []managedPlugin
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if len(got) != 2 || got[0].Name != "broken" || got[0].Error == "" || got[1].Name != "echo" || got[1].Error != "" {
		t.Fatalf("want broken (with its error) then echo, got %+v", got)
	}
	if strings.Join(got[1].Adds, ",") != "api,page,site_tab" {
		t.Errorf("echo adds = %v", got[1].Adds)
	}
	if strings.Join(got[1].Requires, ",") != "/bin/sh" {
		t.Errorf("echo requires = %v, want its scripts' interpreter", got[1].Requires)
	}
}

func TestPluginManager_CatalogMarksWhatThisMachineLacks(t *testing.T) {
	installAPIPlugin(t)
	serveCatalog(t,
		plugins.Listing{Name: "ready", Repo: "https://github.com/x/ready", Commit: strings.Repeat("a", 40), Requires: []string{"sh"}},
		plugins.Listing{Name: "needy", Repo: "https://github.com/x/needy", Commit: strings.Repeat("b", 40), Requires: []string{"sh", "lerd-test-no-such-tool"}},
	)
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/catalog", ""))
	var got []catalogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if len(got) != 2 || len(got[0].Missing) != 0 || strings.Join(got[1].Missing, ",") != "lerd-test-no-such-tool" {
		t.Errorf("got %+v", got)
	}
}

func TestPluginManager_InstallAndRemove(t *testing.T) {
	installAPIPlugin(t)
	src := filepath.Join(t.TempDir(), "src")
	if err := plugins.Scaffold("hello", src); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"source": src})
	handlePluginManager(rec, localRequest(http.MethodPost, "/api/plugin-manager", string(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"hello"`) {
		t.Fatalf("install: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodPost, "/api/plugin-manager", `{"source":"/no/such/dir"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a failed install should answer 400, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodDelete, "/api/plugin-manager/hello", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(plugins.Dir(), "hello")); !os.IsNotExist(err) {
		t.Error("hello should be removed")
	}
}

// Installing runs a clone and later the plugin's code, so the manager is a host
// action: a remote dashboard reaches it only with remote full access.
func TestPluginManager_IsHostOnly(t *testing.T) {
	setupConfigDirFullAccess(t, "alice", "s3cret", false)
	installAPIPlugin(t)
	next := &nextHandler{}
	withRemoteControlGate(next).ServeHTTP(httptest.NewRecorder(), remoteRequest(http.MethodPost, "/api/plugin-manager"))
	if next.called {
		t.Error("a remote dashboard without full access must not install plugins")
	}
}

// pageRequest is what nginx sends lerd-ui for a site page: over the unix
// socket, naming the site, from the page's own origin.
func pageRequest(method, path, site string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKeyUnixSocket{}, true))
	req.Header.Set("X-Lerd-Site", site)
	req.Header.Set("X-Lerd-Host", site+".test")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return req
}

func TestPluginPages_ServeScriptAndEndpointsToSwitchedOnSites(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	installAPIPlugin(t)
	if err := os.WriteFile(filepath.Join(plugins.Dir(), "echo", "ui", "page.js"), []byte("/* page */"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(plugins.Dir(), "echo", plugins.ManifestFile)
	data, _ := os.ReadFile(manifest)
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(data), "ui:\n", "ui:\n  page_script: ui/page.js\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, on := range map[string]bool{"on": true, "off": false} {
		site := config.Site{Name: name, Domains: []string{name + ".test"}, Path: t.TempDir()}
		if on {
			site.PagePlugins = []string{"echo"}
		}
		if err := config.AddSite(site); err != nil {
			t.Fatal(err)
		}
	}
	serve := withPluginPages(http.NotFoundHandler())

	// httptest's default client address is 192.0.2.1, which lerd's own lerd0
	// interface owns on a host running lerd, so a LAN address stands in.
	direct := httptest.NewRequest(http.MethodGet, "/_lerd/plugins/echo/page.js", nil)
	direct.RemoteAddr = "198.51.100.7:40000"
	direct.Header.Set("X-Lerd-Site", "on")

	cases := []struct {
		name string
		req  *http.Request
		want int
		body string
	}{
		{"script", pageRequest(http.MethodGet, "/_lerd/plugins/echo/page.js", "on"), 200, "/* page */"},
		{"endpoint gets the site", pageRequest(http.MethodPost, "/_lerd/plugins/echo/api/say", "on"), 200, `"surface":"api"`},
		{"site never switched it on", pageRequest(http.MethodGet, "/_lerd/plugins/echo/page.js", "off"), 404, ""},
		{"loopback_only stays off the page", pageRequest(http.MethodPost, "/_lerd/plugins/echo/api/host", "on"), 403, ""},
		{"not through nginx", direct, 403, ""},
	}
	cross := pageRequest(http.MethodPost, "/_lerd/plugins/echo/api/say", "on")
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	cases = append(cases, struct {
		name string
		req  *http.Request
		want int
		body string
	}{"another site's page", cross, 403, ""})

	for _, c := range cases {
		rec := httptest.NewRecorder()
		serve.ServeHTTP(rec, c.req)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%s: %d %q, want %d containing %q", c.name, rec.Code, rec.Body, c.want, c.body)
		}
	}
}

func TestPagePlugins_ListsAndRefusesWithoutAuthority(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	installAPIPlugin(t)
	if err := plugins.Scaffold("hello", filepath.Join(plugins.Dir(), "hello")); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir(), PagePlugins: []string{"hello"}}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handlePagePlugins(rec, localRequest(http.MethodGet, "/api/page-plugins?site=shop.test", ""))
	var got []pagePlugin
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	// echo has no page script, so only hello is offered.
	if len(got) != 1 || got[0].Name != "hello" || !got[0].On {
		t.Errorf("got %+v, want hello switched on", got)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/page-plugins", strings.NewReader(`{"site":"shop","plugin":"hello","on":false}`))
	req.RemoteAddr = "198.51.100.7:40000"
	rec = httptest.NewRecorder()
	handlePagePlugins(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a switch from without host authority must be refused, got %d", rec.Code)
	}
}

// serveCatalog points the catalog at a test server listing the given entries.
func serveCatalog(t *testing.T, listings ...plugins.Listing) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(plugins.Catalog{Plugins: listings})
	}))
	t.Cleanup(srv.Close)
	orig := plugins.CatalogURL
	plugins.CatalogURL = srv.URL
	t.Cleanup(func() { plugins.CatalogURL = orig })
}

func TestPluginManager_CatalogMarksInstalled(t *testing.T) {
	installAPIPlugin(t)
	serveCatalog(t,
		plugins.Listing{Name: "echo", Repo: "https://github.com/x/echo", Commit: strings.Repeat("a", 40)},
		plugins.Listing{Name: "other", Repo: "https://github.com/x/other", Commit: strings.Repeat("b", 40), Images: []string{"images/other/i.png"}, Stars: 9},
	)
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/catalog", ""))
	var got []catalogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if len(got) != 2 || !got[0].Installed || got[1].Installed || got[1].Stars != 9 || len(got[1].Images) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestPluginManager_CatalogMarksWhatNeedsANewerLerd(t *testing.T) {
	installAPIPlugin(t)
	serveCatalog(t,
		plugins.Listing{Name: "now", Repo: "https://github.com/x/now", Commit: strings.Repeat("a", 40), Lerd: ">=1.0"},
		plugins.Listing{Name: "later", Repo: "https://github.com/x/later", Commit: strings.Repeat("b", 40), Lerd: ">=999.0"},
	)
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/catalog", ""))
	var got []catalogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if len(got) != 2 || !got[0].Compatible || got[1].Compatible {
		t.Errorf("got %+v", got)
	}
}

// An unreachable catalog answers 502 with the reason rather than an empty
// list, so the page can say why there is nothing to browse.
func TestPluginManager_CatalogUnreachable(t *testing.T) {
	installAPIPlugin(t)
	orig := plugins.CatalogURL
	plugins.CatalogURL = "http://127.0.0.1:1/index.json"
	t.Cleanup(func() { plugins.CatalogURL = orig })
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/catalog", ""))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestPluginManager_InstallByNameGoesToTheCatalog(t *testing.T) {
	installAPIPlugin(t)
	serveCatalog(t)
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodPost, "/api/plugin-manager", `{"name":"nope"}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not in the plugin catalog") {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
}

func TestPluginsList_DescribesUI(t *testing.T) {
	installAPIPlugin(t)
	rec := httptest.NewRecorder()
	handlePluginsList(rec, localRequest(http.MethodGet, "/api/plugins", ""))

	var got []pluginInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if len(got) != 1 || got[0].Name != "echo" {
		t.Fatalf("got %+v", got)
	}
	p := got[0]
	if p.Page == nil || p.Page.URL != "/plugins/echo/ui/index.html" || p.Page.Icon != "/plugins/echo/ui/icon.svg" {
		t.Errorf("page = %+v", p.Page)
	}
	if p.SiteTab == nil || p.SiteTab.URL != "/plugins/echo/ui/site.html" {
		t.Errorf("site tab = %+v", p.SiteTab)
	}
}

func TestPluginEndpoint_StopsAtItsTimeout(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join(plugins.Dir(), "slow")
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestFile), []byte("api_version: 1\nname: slow\napi:\n  - {path: wait, run: bin/wait, timeout: 200ms}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "wait"), []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	rec := httptest.NewRecorder()
	handlePluginAPI(rec, localRequest(http.MethodGet, "/api/plugins/slow/wait", ""))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "timed out after 200ms") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("the endpoint must answer at its limit")
	}
}
