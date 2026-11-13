package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
		"ui/old.htm":    "<p>old</p>",
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
	if n.Kind != "plugin_failed" || n.URL != "#plugins" || n.Params["plugin"] != "echo" || n.Params["message"] != "boom" {
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

// A link under ui/ must not carry the route outside it: the path check is on
// text, and the file the link names could be any the user can read.
func TestPluginUI_RefusesALinkOutOfTheUIDir(t *testing.T) {
	installAPIPlugin(t)
	ui := filepath.Join(plugins.Dir(), "echo", plugins.UIDir)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{"outside": secret, "manifest": filepath.Join("..", plugins.ManifestFile), "inside": "index.html"}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(ui, name)); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]int{"outside": http.StatusNotFound, "manifest": http.StatusNotFound, "inside": http.StatusOK} {
		rec := httptest.NewRecorder()
		handlePluginUI(rec, localRequest(http.MethodGet, "/plugins/echo/ui/"+name, ""))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", name, rec.Code, want)
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
	if got[1].Icon != "/plugins/echo/ui/icon.svg" || got[0].Icon != "" {
		t.Errorf("icons = %q, %q, want echo's page icon only", got[0].Icon, got[1].Icon)
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

// The page script reaches every visitor of the site, so a link out of ui/ is
// refused there too.
func TestPluginPages_RefuseAScriptLinkedOutOfTheUIDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	installAPIPlugin(t)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(plugins.Dir(), "echo", "ui", "page.js")); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(plugins.Dir(), "echo", plugins.ManifestFile)
	data, _ := os.ReadFile(manifest)
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(data), "ui:\n", "ui:\n  page_script: ui/page.js\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.AddSite(config.Site{Name: "on", Domains: []string{"on.test"}, Path: t.TempDir(), PagePlugins: []string{"echo"}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	withPluginPages(http.NotFoundHandler()).ServeHTTP(rec, pageRequest(http.MethodGet, "/_lerd/plugins/echo/page.js", "on"))
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "private") {
		t.Errorf("linked page script = %d %q, want 404", rec.Code, rec.Body)
	}
}

// A page on a worktree's domain calls the plugin in that worktree's checkout,
// and a branch lerd does not know gets nothing rather than the parent's.
func TestPluginPages_EndpointRunsInTheWorktreeCheckout(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	installAPIPlugin(t)
	dir := filepath.Join(plugins.Dir(), "echo")
	if err := os.WriteFile(filepath.Join(dir, "bin", "where"), []byte("#!/bin/sh\npwd -P\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, plugins.ManifestFile)
	data, _ := os.ReadFile(manifest)
	data = []byte(strings.Replace(string(data), "api:\n", "api:\n  - {path: where, methods: [GET], run: bin/where}\n", 1))
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(data), "ui:\n", "ui:\n  page_script: ui/page.js\n", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ui", "page.js"), []byte("/* page */"), 0o644); err != nil {
		t.Fatal(err)
	}
	sitePath := t.TempDir()
	checkout, _ := filepath.EvalSymlinks(t.TempDir())
	makeWorktree(t, sitePath, "feat", "feat", checkout)
	if err := config.AddSite(config.Site{Name: "on", Domains: []string{"on.test"}, Path: sitePath, PagePlugins: []string{"echo"}}); err != nil {
		t.Fatal(err)
	}
	serve := withPluginPages(http.NotFoundHandler())

	req := pageRequest(http.MethodGet, "/_lerd/plugins/echo/api/where", "on")
	req.Header.Set("X-Lerd-Branch", "feat")
	rec := httptest.NewRecorder()
	serve.ServeHTTP(rec, req)
	if got := strings.TrimSpace(rec.Body.String()); rec.Code != 200 || got != checkout {
		t.Errorf("worktree call = %d %q, want 200 in %s", rec.Code, got, checkout)
	}

	req = pageRequest(http.MethodGet, "/_lerd/plugins/echo/api/where", "on")
	req.Header.Set("X-Lerd-Branch", "gone")
	rec = httptest.NewRecorder()
	serve.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown branch = %d %q, want 404", rec.Code, rec.Body)
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
	// echo offers only its tab; hello its script, card and tab.
	if len(got) != 4 || got[0].Name != "echo" || got[0].Kind != kindSiteTab || !got[0].On ||
		got[1].Kind != kindPageScript || !got[1].On || got[2].Kind != kindOverviewCard || got[2].On || got[3].Kind != kindSiteTab || !got[3].On {
		t.Errorf("got %+v, want echo's tab on, hello's script on, its card off and its tab on", got)
	}

	rec = httptest.NewRecorder()
	handlePagePlugins(rec, localRequest(http.MethodPost, "/api/page-plugins", `{"site":"shop","plugin":"hello","kind":"overview_card","on":true}`))
	if site, _ := config.FindSite("shop"); rec.Code != 200 || !slices.Equal(site.OverviewCards, []string{"hello"}) {
		t.Errorf("card switch = %d %s, OverviewCards %v", rec.Code, rec.Body, site.OverviewCards)
	}
	rec = httptest.NewRecorder()
	handlePagePlugins(rec, localRequest(http.MethodPost, "/api/page-plugins", `{"site":"shop","plugin":"hello","kind":"site_tab","on":false}`))
	if site, _ := config.FindSite("shop"); rec.Code != 200 || !slices.Equal(site.HiddenTabs, []string{"hello"}) {
		t.Errorf("tab switch = %d %s, HiddenTabs %v", rec.Code, rec.Body, site.HiddenTabs)
	}
	rec = httptest.NewRecorder()
	handlePagePlugins(rec, localRequest(http.MethodPost, "/api/page-plugins", `{"site":"shop","plugin":"hello","kind":"banner","on":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown kind must be refused, got %d", rec.Code)
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

func TestPluginManager_CatalogMarksWhatRunsElsewhere(t *testing.T) {
	installAPIPlugin(t)
	serveCatalog(t,
		plugins.Listing{Name: "both", Repo: "https://github.com/x/both", Commit: strings.Repeat("a", 40)},
		plugins.Listing{Name: "elsewhere", Repo: "https://github.com/x/elsewhere", Commit: strings.Repeat("b", 40), Platforms: []string{otherPlatform()}},
	)
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/catalog", ""))
	var got []catalogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if len(got) != 2 || !got[0].RunsHere || got[1].RunsHere {
		t.Errorf("got %+v", got)
	}
}

func otherPlatform() string {
	if runtime.GOOS == "darwin" {
		return "linux"
	}
	return "macos"
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

// installDocumentedPlugin adds the echo plugin docs and a screenshot, beside a
// png the manifest does not list.
func installDocumentedPlugin(t *testing.T) string {
	t.Helper()
	installAPIPlugin(t)
	dir := filepath.Join(plugins.Dir(), "echo")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"docs/tab.png":   "png",
		"docs/other.png": "png",
		"DOCS.md":        "# Echo\n\n![tab](docs/tab.png)\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(dir, plugins.ManifestFile)
	data, _ := os.ReadFile(manifest)
	if err := os.WriteFile(manifest, append(data, []byte("docs: DOCS.md\nscreenshots: [docs/tab.png]\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPluginUI_ServesListedScreenshotsOnly(t *testing.T) {
	installDocumentedPlugin(t)
	for path, want := range map[string]int{
		"/plugins/echo/docs/tab.png":   http.StatusOK,
		"/plugins/echo/docs/other.png": http.StatusNotFound,
		"/plugins/echo/DOCS.md":        http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		handlePluginUI(rec, localRequest(http.MethodGet, path, ""))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
}

// A listed screenshot that is a link must not reach a file outside the plugin.
func TestPluginUI_RefusesAScreenshotLinkedOutOfThePlugin(t *testing.T) {
	dir := installDocumentedPlugin(t)
	secret := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := filepath.Join(dir, "docs", "tab.png")
	if err := os.Remove(tab); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, tab); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handlePluginUI(rec, localRequest(http.MethodGet, "/plugins/echo/docs/tab.png", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("linked screenshot = %d, want 404", rec.Code)
	}
}

func TestPluginManager_DescribesFeaturesScreenshotsAndDocs(t *testing.T) {
	installDocumentedPlugin(t)
	rows := managedPlugins()
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	r := rows[0]
	if !r.HasDocs || strings.Join(r.Screenshots, ",") != "/plugins/echo/docs/tab.png" {
		t.Errorf("has_docs = %v, screenshots = %v", r.HasDocs, r.Screenshots)
	}
	var kinds []string
	for _, f := range r.Features {
		kinds = append(kinds, f.Kind+":"+f.Name)
	}
	if strings.Join(kinds, ",") != "page:Echo,site_tab:Echo" {
		t.Errorf("features = %v", kinds)
	}

	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/echo/docs", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `/plugins/echo/docs/tab.png`) || !strings.Contains(rec.Body.String(), "Echo") {
		t.Errorf("docs = %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/nope/docs", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown plugin docs = %d, want 404", rec.Code)
	}
}

func TestPluginManager_LogsListRecentFailures(t *testing.T) {
	installAPIPlugin(t)
	plugins.Record(plugins.Failure{Plugin: "echo", Surface: plugins.SurfaceAPI, What: "say", Error: "first"})
	plugins.Record(plugins.Failure{Plugin: "echo", Surface: plugins.SurfaceHook, What: "site.linked", Error: "second"})
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/echo/logs", ""))
	var got []pluginFailure
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 2 || got[0].Error != "second" {
		t.Fatalf("logs = %d %s", rec.Code, rec.Body)
	}
}

func TestInjectPluginKit(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"after head":         {"<!doctype html><html><head><title>x</title></head>", "<head>" + pluginKitTags + "<title>"},
		"head with attrs":    {"<HEAD lang=\"en\"><style>b{}</style>", "<HEAD lang=\"en\">" + pluginKitTags + "<style>"},
		"no head":            {"<p>bare</p>", pluginKitTags + "<p>bare</p>"},
		"header is not head": {"<header>x</header>", pluginKitTags + "<header>"},
	} {
		if got := string(injectPluginKit([]byte(c.in))); !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want it to contain %q", name, got, c.want)
		}
	}
}

// Every page gets lerd's look and theme ahead of its own styles; other files
// are served as they are.
func TestPluginUI_AddsTheKitToPagesOnly(t *testing.T) {
	installAPIPlugin(t)
	rec := httptest.NewRecorder()
	handlePluginUI(rec, localRequest(http.MethodGet, "/plugins/echo/ui/index.html", ""))
	if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.HasPrefix(body, pluginKitTags) || !strings.Contains(body, "<p>echo</p>") {
		t.Errorf("page = %d %q", rec.Code, body)
	}
	rec = httptest.NewRecorder()
	handlePluginUI(rec, localRequest(http.MethodGet, "/plugins/echo/ui/old.htm", ""))
	if !strings.HasPrefix(rec.Body.String(), pluginKitTags) {
		t.Errorf("an .htm page got no kit: %q", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handlePluginUI(rec, localRequest(http.MethodGet, "/plugins/echo/ui/icon.svg", ""))
	if rec.Body.String() != "<svg/>" {
		t.Errorf("icon changed: %q", rec.Body.String())
	}
	for path, kind := range map[string]string{"/plugins/_lerd/base.css": "text/css", "/plugins/_lerd/theme.js": "javascript"} {
		rec = httptest.NewRecorder()
		handlePluginUI(rec, localRequest(http.MethodGet, path, ""))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), kind) || rec.Body.Len() == 0 {
			t.Errorf("%s = %d %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

// The kit gives buttons a display of their own, which would otherwise outrank
// the browser's [hidden] rule and keep a hidden button on screen.
func TestPluginKit_KeepsHiddenElementsHidden(t *testing.T) {
	css, err := pluginKit.ReadFile("pluginkit/base.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "[hidden] { display: none !important; }") {
		t.Error("base.css must restore [hidden] over its own display rules")
	}
}

// A plugin that stops loading keeps its failure log, the one place that says why.
func TestPluginManager_LogsOfABrokenPlugin(t *testing.T) {
	installAPIPlugin(t)
	plugins.Record(plugins.Failure{Plugin: "echo", Surface: plugins.SurfaceAPI, What: "say", Error: "boom"})
	if err := os.WriteFile(filepath.Join(plugins.Dir(), "echo", plugins.ManifestFile), []byte("api_version: 9\nname: echo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/echo/logs", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("logs = %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	handlePluginManager(rec, localRequest(http.MethodGet, "/api/plugin-manager/..%2Fx/logs", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("a name that is not a plugin's = %d, want 404", rec.Code)
	}
}
