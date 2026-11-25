package ui

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/stats"
)

// stubSuggestion gives every named service the same memory.
func stubSuggestion(t *testing.T, sleepable []string, memEach int64) {
	t.Helper()
	isolateThemesDir(t)
	running, sleep, st := suggestRunning, suggestSleepable, suggestStats
	t.Cleanup(func() { suggestRunning, suggestSleepable, suggestStats = running, sleep, st })
	suggestRunning = func() []string { return sleepable }
	suggestSleepable = func(names []string, _ map[string]int64, _ time.Duration, _ time.Time) []string { return names }
	suggestStats = func() stats.Snapshot {
		// A container that is not a candidate never counts toward the saving.
		snap := stats.Snapshot{Available: true, Containers: []stats.ContainerStat{{Name: "lerd-nginx", MemBytes: 4 << 30}}}
		for _, s := range sleepable {
			snap.Containers = append(snap.Containers, stats.ContainerStat{Name: "lerd-" + s, MemBytes: memEach})
		}
		return snap
	}
}

func TestIdleServicesSuggestion(t *testing.T) {
	three := []string{"meilisearch", "mysql", "redis"}
	cases := []struct {
		name      string
		sleepable []string
		mem       int64
		set       func(*config.GlobalConfig)
		want      bool
	}{
		{"enough services and memory", three, 200 << 20, nil, true},
		{"too few services", three[:2], 2 << 30, nil, false},
		{"too little memory", three, 10 << 20, nil, false},
		{"already on", three, 200 << 20, func(c *config.GlobalConfig) { c.SetIdleServices(true) }, false},
		{"turned off by hand", three, 200 << 20, func(c *config.GlobalConfig) { c.SetIdleServices(false) }, false},
		{"idle-suspend off", three, 200 << 20, func(c *config.GlobalConfig) { c.IdleSuspend.Enabled = false }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubSuggestion(t, tc.sleepable, tc.mem)
			if tc.set != nil {
				cfg, err := config.LoadGlobal()
				if err != nil {
					t.Fatal(err)
				}
				tc.set(cfg)
				if err := config.SaveGlobal(cfg); err != nil {
					t.Fatal(err)
				}
			}
			got := idleServicesSuggestion(time.Now())
			if !tc.want {
				if len(got.Services) != 0 {
					t.Fatalf("suggested %+v", got)
				}
				return
			}
			if !reflect.DeepEqual(got.Services, three) || got.MemBytes != 600<<20 {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestIdleServicesSuggestionNeedsReadableStats(t *testing.T) {
	stubSuggestion(t, []string{"mysql", "redis", "meilisearch"}, 200<<20)
	suggestStats = func() stats.Snapshot { return stats.Snapshot{} }
	if got := idleServicesSuggestion(time.Now()); len(got.Services) != 0 {
		t.Fatalf("suggested %+v with no memory reading", got)
	}
}

func TestIdleServicesSuggestionEndpointRefusesPost(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIdleServicesSuggestion(rec, httptest.NewRequest(http.MethodPost, "/api/idle/services-suggestion", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", rec.Code)
	}
}

// Dismissing the banner is the dashboard saving services as off.
func TestSavingServicesOffDeclinesTheSuggestion(t *testing.T) {
	stubSuggestion(t, []string{"mysql", "redis", "meilisearch"}, 200<<20)
	rec := httptest.NewRecorder()
	handleSettingsIdleSuspend(rec, httptest.NewRequest(http.MethodPost, "/api/settings/idle-suspend",
		strings.NewReader(`{"enabled":true,"timeout_minutes":30,"services":false}`)))
	if got := idleServicesSuggestion(time.Now()); len(got.Services) != 0 {
		t.Fatalf("still suggested after a decline: %+v", got)
	}
}

// Saving only the timeout says nothing about services.
func TestSavingTheTimeoutDoesNotDecline(t *testing.T) {
	stubSuggestion(t, []string{"mysql", "redis", "meilisearch"}, 200<<20)
	rec := httptest.NewRecorder()
	handleSettingsIdleSuspend(rec, httptest.NewRequest(http.MethodPost, "/api/settings/idle-suspend",
		strings.NewReader(`{"enabled":true,"timeout_minutes":45}`)))
	if got := idleServicesSuggestion(time.Now()); len(got.Services) != 3 {
		t.Fatalf("a timeout change dropped the suggestion: %+v", got)
	}
}
