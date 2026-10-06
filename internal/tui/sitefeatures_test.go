package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/geodro/lerd/internal/config"
	lerddumps "github.com/geodro/lerd/internal/dumps"
	"github.com/geodro/lerd/internal/siteinfo"
)

func featureSite() siteinfo.EnrichedSite {
	return siteinfo.EnrichedSite{
		Name: "shop", Domains: []string{"shop.test"}, Path: "/p/shop", PHPVersion: "8.4",
		FPMRunning: true, HasHorizon: true, StripeSecretSet: true,
	}
}

func rowKindSet(rows []detailRow) map[detailKind]bool {
	out := map[detailKind]bool{}
	for _, r := range rows {
		out[r.kind] = true
	}
	return out
}

func TestDetailRowsOfferTheSiteToggles(t *testing.T) {
	s := featureSite()
	k := rowKindSet(detailRows(&s))
	for kind, name := range map[detailKind]string{kindPin: "pin", kindRuntime: "runtime", kindHorizonReload: "horizon reload", kindStripe: "stripe"} {
		if !k[kind] {
			t.Errorf("site toggles missing %s", name)
		}
	}

	bare := siteinfo.EnrichedSite{Name: "static", Path: "/p/static", ContainerPort: 8080}
	k = rowKindSet(detailRows(&bare))
	if k[kindRuntime] || k[kindHorizonReload] || k[kindStripe] {
		t.Error("a custom-container site without horizon or stripe should only get the toggles that apply")
	}
	if !k[kindPin] {
		t.Error("every site can be kept awake")
	}
}

func toggleRow(t *testing.T, m *Model, kind detailKind) tea.Cmd {
	t.Helper()
	s := m.currentSite()
	rows := detailRows(s)
	nav := navigableRows(rows)
	for pos, i := range nav {
		if rows[i].kind == kind {
			m.detailCursor = pos
			return m.detailToggleSelected(s, rows, nav)
		}
	}
	t.Fatalf("row kind %d not navigable", kind)
	return nil
}

func featureModel() *Model {
	m := NewModel("test")
	m.snap = Snapshot{Sites: []siteinfo.EnrichedSite{featureSite()}}
	m.switchTab(tabSites)
	m.focusMain()
	return m
}

func TestTogglePinKeepsTheSiteAwake(t *testing.T) {
	m := featureModel()
	if toggleRow(t, m, kindPin) == nil || !strings.Contains(m.status, "keeping shop awake") {
		t.Fatalf("pin should run and say so, status %q", m.status)
	}
	m.snap.Pinned = map[string]bool{"shop": true}
	if toggleRow(t, m, kindPin) == nil || !strings.Contains(m.status, "letting shop idle") {
		t.Fatalf("unpin should run and say so, status %q", m.status)
	}
}

func TestToggleRuntimeSwitchesBetweenFPMAndFrankenPHP(t *testing.T) {
	m := featureModel()
	if toggleRow(t, m, kindRuntime) == nil || !strings.Contains(m.status, "frankenphp") {
		t.Fatalf("an fpm site should switch to frankenphp, status %q", m.status)
	}
	m.snap.Sites[0].Runtime = "frankenphp"
	if toggleRow(t, m, kindRuntime) == nil || !strings.Contains(m.status, "php-fpm") {
		t.Fatalf("a frankenphp site should switch back to php-fpm, status %q", m.status)
	}
}

func TestToggleHorizonReloadAndStripe(t *testing.T) {
	m := featureModel()
	if toggleRow(t, m, kindHorizonReload) == nil || !strings.Contains(m.status, "reload horizon") {
		t.Fatalf("horizon reload should turn on, status %q", m.status)
	}
	if toggleRow(t, m, kindStripe) == nil || !strings.Contains(m.status, "stripe listener") {
		t.Fatalf("stripe listener should start, status %q", m.status)
	}
}

func TestEditorAndFolderKeys(t *testing.T) {
	m := featureModel()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	m = next.(*Model)
	if cmd == nil || !strings.Contains(m.status, "editor") {
		t.Fatalf("E should open the site in the editor, status %q", m.status)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	m = next.(*Model)
	if cmd == nil || !strings.Contains(m.status, "/p/shop") {
		t.Fatalf("F should open the site folder, status %q", m.status)
	}
}

func TestOverviewListsSuggestedServices(t *testing.T) {
	m := featureModel()
	m.snap.Sites[0].SuggestedServices = []config.ServiceSuggestion{{Name: "meilisearch", Package: "laravel/scout"}}
	lines, _ := detailContentLines(m, m.currentSite(), true, 100)
	out := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(out, "Suggested") || !strings.Contains(out, "meilisearch") || !strings.Contains(out, "laravel/scout") {
		t.Fatalf("overview should list the suggestion and why:\n%s", out)
	}
}

func TestDebugLensesCoverLogsExceptionsAndMessages(t *testing.T) {
	have := map[string]bool{}
	for _, l := range debugLenses {
		have[l.kind] = true
	}
	for _, k := range []string{lerddumps.KindLog, lerddumps.KindException, lerddumps.KindMessage} {
		if !have[k] {
			t.Errorf("missing lens for %q", k)
		}
	}
}

func TestDebugRowMainForTheNewKinds(t *testing.T) {
	ev := func(kind string, data map[string]any) lerddumps.Event {
		raw, _ := json.Marshal(data)
		return lerddumps.Event{Kind: kind, Data: raw}
	}
	cases := []struct {
		kind string
		data map[string]any
		want []string
	}{
		{lerddumps.KindLog, map[string]any{"message": "Order placed", "level": "error", "channel": "stack"}, []string{"Order placed", "error", "stack"}},
		{lerddumps.KindException, map[string]any{"type": "RuntimeException", "message": "boom", "level": "error"}, []string{"RuntimeException", "boom"}},
		{lerddumps.KindMessage, map[string]any{"body": "Your code is 1234", "to": "+40700", "channel": "sms"}, []string{"Your code is 1234", "+40700", "sms"}},
	}
	for _, c := range cases {
		got := stripANSI(debugRowMain(c.kind, ev(c.kind, c.data), nil))
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s row %q missing %q", c.kind, got, w)
			}
		}
	}
}
