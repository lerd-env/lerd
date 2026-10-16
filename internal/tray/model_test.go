package tray

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

const runningStatus = `{"nginx":{"running":true},"dns":{"ok":true,"enabled":true},"php_default":"8.4"}`

func runningSnapshot() *Snapshot {
	return &Snapshot{
		Running:       true,
		NginxRunning:  true,
		DNSOK:         true,
		PHPVersions:   []phpInfo{{Version: "8.3"}, {Version: "8.4"}},
		PHPDefault:    "8.4",
		ServicesKnown: true,
		Services: []serviceInfo{
			{Name: "mysql", Status: "active"},
			{Name: "redis", Status: "inactive"},
			{Name: "meilisearch", Status: "inactive", Paused: true},
		},
		WorkersRunning:       2,
		NotificationsEnabled: true,
		LatestVersion:        "v9.9.9",
	}
}

// find returns the first item, at any depth, whose label starts with prefix.
func find(items []MenuItem, prefix string) (MenuItem, bool) {
	for _, it := range items {
		if strings.HasPrefix(it.Label, prefix) {
			return it, true
		}
		if sub, ok := find(it.Items, prefix); ok {
			return sub, true
		}
	}
	return MenuItem{}, false
}

func mustFind(t *testing.T, m Menu, prefix string) MenuItem {
	t.Helper()
	it, ok := find(m.Items, prefix)
	if !ok {
		t.Fatalf("no menu item starting with %q in %+v", prefix, m.Items)
	}
	return it
}

func TestBuildMenu_DescribesStateWithoutDrawingIt(t *testing.T) {
	m := BuildMenu(runningSnapshot(), MenuOptions{LANToggle: true})

	if !m.Running {
		t.Error("menu should report lerd running")
	}
	if it := mustFind(t, m, "Running"); it.State != StateRunning || it.Action != "" {
		t.Errorf("status item = %+v, want an inert running row", it)
	}
	if it := mustFind(t, m, "2 workers"); it.State != StateRunning {
		t.Errorf("workers item = %+v", it)
	}
	for _, it := range []MenuItem{mustFind(t, m, "Running"), mustFind(t, m, "nginx"), mustFind(t, m, "mysql")} {
		if strings.ContainsAny(it.Label, "🟢🔴🟡⚪") {
			t.Errorf("label %q carries a drawn dot; the state field is for that", it.Label)
		}
	}
}

func TestBuildMenu_ServicesCarryStateAndTheirAction(t *testing.T) {
	m := BuildMenu(runningSnapshot(), MenuOptions{})

	if it := mustFind(t, m, "Services"); it.Label != "Services (1/3)" || len(it.Items) != 3 {
		t.Fatalf("services section = %+v", it)
	}
	cases := map[string]struct {
		state  State
		action string
	}{
		"mysql":       {StateRunning, "service stop mysql"},
		"redis":       {StateStopped, "service start redis"},
		"meilisearch": {StateWarning, "service start meilisearch"},
	}
	for name, want := range cases {
		it := mustFind(t, m, name)
		if it.State != want.state || it.Action != want.action {
			t.Errorf("%s = %+v, want state %q action %q", name, it, want.state, want.action)
		}
	}
}

func TestBuildMenu_PHPAndTogglesAreChecked(t *testing.T) {
	m := BuildMenu(runningSnapshot(), MenuOptions{LANToggle: true})

	if it := mustFind(t, m, "8.4"); it.Checked == nil || !*it.Checked || it.Action != "use 8.4" {
		t.Errorf("default PHP = %+v, want checked", it)
	}
	if it := mustFind(t, m, "8.3"); it.Checked == nil || *it.Checked {
		t.Errorf("other PHP = %+v, want unchecked", it)
	}
	if it := mustFind(t, m, "Notifications"); it.Checked == nil || !*it.Checked || it.Action != "notify off" {
		t.Errorf("notifications toggle = %+v", it)
	}
	if it := mustFind(t, m, "Autostart"); it.Checked == nil || *it.Checked || it.Action != "autostart enable" {
		t.Errorf("autostart toggle = %+v", it)
	}
	if it := mustFind(t, m, "Update to v9.9.9"); it.Action != "update" {
		t.Errorf("update item = %+v", it)
	}
	if _, ok := find(m.Items, "Turn off"); ok {
		t.Error("an API client keeps its tray, so the menu must not offer to turn it off")
	}
}

func TestBuildMenu_LANToggleOnlyWhereSupported(t *testing.T) {
	if _, ok := find(BuildMenu(runningSnapshot(), MenuOptions{}).Items, "Expose to LAN"); ok {
		t.Error("LAN toggle shown without MenuOptions.LANToggle")
	}
	if _, ok := find(BuildMenu(runningSnapshot(), MenuOptions{LANToggle: true}).Items, "Expose to LAN"); !ok {
		t.Error("LAN toggle missing with MenuOptions.LANToggle")
	}
}

func TestBuildMenu_ArmedServiceLANAccessSaysWhy(t *testing.T) {
	snap := runningSnapshot()
	snap.LANServicesExposed = true
	it := mustFind(t, BuildMenu(snap, MenuOptions{}), "Managed service LAN access")
	if it.Checked == nil || !*it.Checked || !strings.Contains(it.Detail, "LAN exposure off") {
		t.Errorf("armed toggle = %+v", it)
	}
}

func TestBuildMenu_StoppedOffersStart(t *testing.T) {
	m := BuildMenu(&Snapshot{ServicesKnown: true}, MenuOptions{})
	if m.Running {
		t.Error("menu should report lerd stopped")
	}
	if it := mustFind(t, m, "Start Lerd"); it.Action != "start" {
		t.Errorf("toggle = %+v", it)
	}
	if _, ok := find(m.Items, "Services"); ok {
		t.Error("an empty services section should be left out")
	}
}

func TestMenuFind_OnlyResolvesWhatTheMenuOffers(t *testing.T) {
	m := BuildMenu(runningSnapshot(), MenuOptions{})

	if it, ok := m.Find("service start redis"); !ok || strings.Join(it.Args, " ") != "service start redis" {
		t.Errorf("Find(service start redis) = %+v, %v", it, ok)
	}
	if it, ok := m.Find("update"); !ok || it.Args != nil {
		t.Errorf("Find(update) = %+v, %v; want the update item with no lerd command", it, ok)
	}
	for _, id := range []string{"", "service stop redis", "notify on", "uninstall", "service start redis; rm -rf ~"} {
		if it, ok := m.Find(id); ok {
			t.Errorf("Find(%q) = %+v; the menu does not offer it", id, it)
		}
	}
}

func TestMenu_JSONShape(t *testing.T) {
	b, err := json.Marshal(BuildMenu(runningSnapshot(), MenuOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"running":true`, `"label":"mysql"`, `"state":"running"`, `"action":"service stop mysql"`, `"checked":true`} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON lacks %s: %s", want, s)
		}
	}
	if strings.Contains(s, `"args"`) || strings.Contains(s, `"Args"`) {
		t.Errorf("JSON leaks the lerd command behind an action: %s", s)
	}
}

func TestReadSnapshot_ThroughAGetter(t *testing.T) {
	bodies := map[string]string{
		"/api/status":         runningStatus,
		"/api/services":       `[{"name":"mysql","status":"active"},{"name":"queue-acme","status":"active","queue_site":"acme"}]`,
		"/api/workers/health": `{"unhealthy":[]}`,
	}
	get := func(path string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(bodies[path])), nil
	}
	snap := readSnapshot(get, func() Settings { return Settings{Dumps: true} })

	if !snap.Running || len(snap.Services) != 1 || snap.WorkersRunning != 1 || !snap.DumpsEnabled {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestReadSnapshot_DownWhenStatusFails(t *testing.T) {
	read := false
	get := func(string) (io.ReadCloser, error) { return nil, errors.New("refused") }
	snap := readSnapshot(get, func() Settings { read = true; return Settings{} })

	if snap.Running || !snap.ServicesKnown || read {
		t.Errorf("snapshot = %+v, settings read = %v; want a stopped lerd and no settings read", snap, read)
	}
}

func TestOffOn_FlipsState(t *testing.T) {
	if offOn(true) != "off" || offOn(false) != "on" {
		t.Errorf("offOn(true), offOn(false) = %q, %q; want off, on", offOn(true), offOn(false))
	}
}

func TestActionMessage(t *testing.T) {
	fail := errors.New("exit status 1")
	cases := []struct {
		args        []string
		err         error
		stderr      string
		title, body string
		show        bool
	}{
		{[]string{"start"}, nil, "", "Lerd started", "", true},
		{[]string{"stop"}, nil, "", "Lerd stopped", "", true},
		{[]string{"quit"}, nil, "", "Lerd stopped", "", true},
		{[]string{"service", "start", "mailpit"}, nil, "", "mailpit started", "", true},
		{[]string{"use", "8.4"}, nil, "", "PHP 8.4 is the default", "", true},
		{[]string{"autostart", "enable"}, nil, "", "Autostart on", "", true},
		{[]string{"lan", "unexpose"}, nil, "", "LAN access off", "", true},
		{[]string{"lan", "services", "on"}, nil, "", "Services on the LAN on", "", true},
		{[]string{"dump", "off"}, nil, "", "Dumps off", "", true},
		{[]string{"notify", "on"}, nil, "", "Notifications on", "", true},
		{[]string{"tray", "icon", "high-contrast"}, nil, "", "Tray icon: high-contrast", "", true},
		{[]string{"dashboard"}, nil, "", "", "", false},
		{[]string{"dashboard"}, fail, "", "Couldn't open the dashboard", "exit status 1", true},
		{[]string{"service", "start", "redis"}, fail, "pulling image\nError: port 6379 in use\n\n", "Couldn't start redis", "Error: port 6379 in use", true},
		{[]string{"frobnicate"}, nil, "", "lerd frobnicate done", "", true},
	}
	for _, c := range cases {
		title, body, show := ActionMessage(c.args, c.err, c.stderr)
		if title != c.title || body != c.body || show != c.show {
			t.Errorf("ActionMessage(%v, %v) = %q, %q, %v; want %q, %q, %v", c.args, c.err, title, body, show, c.title, c.body, c.show)
		}
	}
}
