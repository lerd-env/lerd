package browsercapture

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/dumps"
)

func TestScript_BakesInTheSiteSettings(t *testing.T) {
	js := Script(config.BrowserCaptureSettings{Enabled: true, Console: []string{"error", "warn"}, Network: []string{"5xx"}, Route: "/__dev/capture"})
	if strings.Contains(js, "__LERD_CONFIG__") {
		t.Fatal("config placeholder left in the script")
	}
	if !strings.Contains(js, `{"console":["error","warn"],"endpoint":"/__dev/capture","events":null,"navigation":false,"network":["5xx"],"resources":false,"verbose":false}`) {
		t.Fatalf("settings missing from script:\n%s", js)
	}
}

func TestScript_IsIgnoreListedForDevTools(t *testing.T) {
	js := Script(config.BrowserCaptureSettings{Enabled: true})
	i := strings.LastIndex(js, "base64,")
	if i < 0 {
		t.Fatal("no inline source map")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(js[i+len("base64,"):]))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		IgnoreList []int  `json:"ignoreList"`
		Mappings   string `json:"mappings"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || len(m.IgnoreList) != 1 || m.IgnoreList[0] != 0 {
		t.Fatalf("source map = %s (%v)", raw, err)
	}
	if got, want := strings.Count(m.Mappings, ";")+1, strings.Count(js, "\n")-1; got != want {
		t.Fatalf("map covers %d lines, script has %d", got, want)
	}
}

func TestScript_OffForTheSiteIsEmpty(t *testing.T) {
	js := Script(config.BrowserCaptureSettings{Enabled: false, Console: []string{"error"}})
	if strings.Contains(js, "addEventListener") {
		t.Fatalf("a site with capture off must get no capture code:\n%s", js)
	}
}

func TestEvents_TagsTheSiteNginxNamedAndDropsUnknownTypes(t *testing.T) {
	body := `[
		{"type":"error","message":"Uncaught TypeError: x is undefined","file":"https://shop.test/app.js","line":12,"url":"https://shop.test/cart","page":"p1"},
		{"type":"telemetry","message":"not ours"},
		{"type":"network","message":"500 POST /api/cart","status":500}
	]`
	evs, err := Events([]byte(body), "shop", "feature-x", "feature-x.shop.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2", len(evs))
	}
	e := evs[0]
	if !e.Valid() || e.Kind != dumps.KindBrowser || e.Label != "error" {
		t.Fatalf("bad event: %+v", e)
	}
	if e.Ctx.Site != "shop" || e.Ctx.Branch != "feature-x" || e.Ctx.Domain != "feature-x.shop.test" || e.Ctx.Request != "https://shop.test/cart" || e.Ctx.RID != "p1" {
		t.Fatalf("bad context: %+v", e.Ctx)
	}
	if e.Src.File != "https://shop.test/app.js" || e.Src.Line != 12 {
		t.Fatalf("bad source: %+v", e.Src)
	}
	var r Report
	if err := json.Unmarshal(evs[1].Data, &r); err != nil || r.Status != 500 {
		t.Fatalf("network data = %s (%v)", evs[1].Data, err)
	}
}

func TestEvents_RefusesMalformedBodyAndCapsBatch(t *testing.T) {
	if _, err := Events([]byte("{"), "shop", "", "shop.test"); err == nil {
		t.Fatal("expected a decode error")
	}
	many := "[" + strings.TrimSuffix(strings.Repeat(`{"type":"error","message":"boom"},`, MaxReports+10), ",") + "]"
	evs, err := Events([]byte(many), "shop", "", "shop.test")
	if err != nil || len(evs) != MaxReports {
		t.Fatalf("got %d events (%v), want %d", len(evs), err, MaxReports)
	}
}

func TestSetEnabled_RegeneratesVhostsAndReloads(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var regenerated, reloaded int
	origRegen, origReload := regenerateVhostsFn, nginxReloadFn
	regenerateVhostsFn = func() error { regenerated++; return nil }
	nginxReloadFn = func() error { reloaded++; return nil }
	t.Cleanup(func() {
		regenerateVhostsFn, nginxReloadFn = origRegen, origReload
	})

	res, err := SetEnabled(true)
	if err != nil || !res.Enabled || res.NoChange {
		t.Fatalf("SetEnabled(true) = %+v, %v", res, err)
	}
	cfg, _ := config.LoadGlobal()
	if !cfg.IsBrowserCaptureEnabled() {
		t.Fatal("toggle not saved")
	}
	if res, _ := SetEnabled(true); !res.NoChange {
		t.Fatal("second call should be a no-op")
	}
	if regenerated != 1 || reloaded != 1 {
		t.Fatalf("regenerated %d, reloaded %d, want 1 and 1", regenerated, reloaded)
	}
}

func TestSaveSite_OnlyANewRouteRewritesTheVhost(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg, _ := config.LoadGlobal()
	cfg.BrowserCapture.Enabled = true
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatal(err)
	}
	site := config.Site{Name: "shop", Domains: []string{"shop.test"}, Path: t.TempDir()}
	if err := config.AddSite(site); err != nil {
		t.Fatal(err)
	}
	var rewritten []string
	origRegen, origReload := regenerateSiteVhostFn, nginxReloadFn
	regenerateSiteVhostFn = func(s config.Site) error { rewritten = append(rewritten, s.Name); return nil }
	nginxReloadFn = func() error { return nil }
	t.Cleanup(func() { regenerateSiteVhostFn, nginxReloadFn = origRegen, origReload })

	s := config.BrowserCaptureSettings{Enabled: true, Console: []string{"warn"}, Network: []string{}, Route: config.DefaultBrowserCaptureRoute}
	if err := SaveSite(site, s); err != nil {
		t.Fatal(err)
	}
	if len(rewritten) != 0 {
		t.Fatalf("console change rewrote the vhost: %v", rewritten)
	}
	s.Route = "/__dev/capture"
	if err := SaveSite(site, s); err != nil {
		t.Fatal(err)
	}
	if len(rewritten) != 1 || rewritten[0] != "shop" {
		t.Fatalf("route change rewrote %v, want [shop]", rewritten)
	}
}

func TestPresets_DetectedFirstThenByLabel(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(config.StoreIndexFile()), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(config.StoreIndexFile(), []byte(`{"frameworks":[],"npm_packages":[{"name":"vue"},{"name":"htmx"},{"name":"alpinejs"},{"name":"@inertiajs/vue3"}]}`), 0644) //nolint:errcheck
	for pkg, preset := range map[string][2]string{"vue": {"vue", "Vue"}, "htmx": {"htmx", "htmx"}, "alpinejs": {"alpine", "Alpine.js"}, "@inertiajs/vue3": {"inertia", "Inertia"}} {
		p := &config.FrameworkPackage{Package: pkg, Type: config.PackageNPM, Browser: &config.PackageBrowser{Preset: preset[0], Label: preset[1], Events: []config.BrowserCaptureEvent{{Event: preset[0] + ":error"}}}}
		if err := config.SaveStorePackage(p); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"vue":"^3","htmx":"^2"}}`), 0644) //nolint:errcheck
	var got []string
	for _, p := range Presets(config.Site{Name: "shop", Path: dir}) {
		got = append(got, p.Name)
	}
	if strings.Join(got, ",") != "htmx,vue,alpine,inertia" {
		t.Fatalf("order = %v", got)
	}
}

func TestCapturable_CoversProxySitesButNotSleepingOnes(t *testing.T) {
	cases := []struct {
		site config.Site
		want bool
	}{
		{config.Site{Name: "fpm"}, true},
		{config.Site{Name: "spa", HostPort: 5173}, true},
		{config.Site{Name: "asleep", HostPort: 5173, IdleSuspendedWorkers: []string{config.HostProxyWorkerName}}, false},
		{config.Site{Name: "paused", Paused: true}, false},
	}
	for _, c := range cases {
		if got := Capturable(c.site); got != c.want {
			t.Errorf("Capturable(%s) = %v, want %v", c.site.Name, got, c.want)
		}
	}
}

func TestSummarize_GroupsPerPageViewAndFiltersByType(t *testing.T) {
	ev := func(id, rid, ts string, r Report) dumps.Event {
		data, _ := json.Marshal(r)
		return dumps.Event{V: 1, ID: id, TS: ts, Kind: dumps.KindBrowser, Ctx: dumps.Context{Type: "browser", RID: rid, Request: r.URL}, Data: data}
	}
	events := []dumps.Event{
		ev("1", "p1", "t1", Report{Type: "navigation", Message: "https://shop.test/", URL: "https://shop.test/"}),
		ev("2", "p1", "t2", Report{Type: "error", Message: "boom", File: "https://shop.test/app.js", Line: 12, Col: 5, URL: "https://shop.test/"}),
		ev("3", "p2", "t3", Report{Type: "console", Level: "warn", Message: "meh", URL: "https://shop.test/cart"}),
		ev("4", "p2", "t4", Report{Type: "network", Message: "500 POST /api/cart", Method: "POST", Request: "/api/cart", Status: 500, URL: "https://shop.test/cart"}),
		{V: 1, ID: "5", Kind: dumps.KindQuery},
	}

	all := Summarize(events, nil)
	if len(all.PageViews) != 2 || all.PageViews[0].URL != "https://shop.test/cart" {
		t.Fatalf("page views = %+v", all.PageViews)
	}
	if all.Counts["console.warn"] != 1 || all.Counts["navigation"] != 1 || len(all.Counts) != 4 {
		t.Fatalf("counts = %v", all.Counts)
	}
	if got := all.PageViews[1].Events[1].At; got != "https://shop.test/app.js:12:5" {
		t.Fatalf("at = %q", got)
	}

	errs := Summarize(events, []string{"error", "network"})
	if len(errs.PageViews) != 2 || len(errs.PageViews[0].Events) != 1 || errs.PageViews[0].Events[0].Status != 500 {
		t.Fatalf("filtered = %+v", errs.PageViews)
	}
}

func TestEventTime_UsesTheBrowserClockWhenItIsClose(t *testing.T) {
	now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	if got := eventTime("2026-10-04T09:59:59.700Z", now); got != "2026-10-04T09:59:59.700Z" {
		t.Fatalf("close clock = %q", got)
	}
	if got := eventTime("2020-01-01T00:00:00Z", now); got != "2026-10-04T10:00:00.000Z" {
		t.Fatalf("far clock = %q", got)
	}
	if got := eventTime("", now); got != "2026-10-04T10:00:00.000Z" {
		t.Fatalf("no clock = %q", got)
	}
}
