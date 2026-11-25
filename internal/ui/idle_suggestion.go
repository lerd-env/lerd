package ui

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/stats"
)

// Below either of these, sleeping the services saves too little to ask about.
const (
	suggestMinServices = 3
	suggestMinMemBytes = 400 << 20
)

// IdleServicesSuggestion names the services sleeping services would stop now and
// the memory they hold. Empty when there is nothing worth suggesting.
type IdleServicesSuggestion struct {
	Services []string `json:"services"`
	MemBytes int64    `json:"mem_bytes"`
}

// The running services and their memory, seams so tests need no containers.
var (
	suggestRunning   = cli.RunningServicesForIdle
	suggestSleepable = cli.SleepableServices
	suggestStats     = func() stats.Snapshot { return stats.Cached(stats.CacheTTL) }
)

func handleIdleServicesSuggestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, idleServicesSuggestion(time.Now()))
}

func idleServicesSuggestion(now time.Time) IdleServicesSuggestion {
	none := IdleServicesSuggestion{Services: []string{}}
	cfg, err := config.LoadGlobal()
	if err != nil || !cfg.IdleSuspend.Enabled || cfg.IdleSuspend.Services || cfg.IdleSuspend.ServicesDeclined {
		return none
	}
	names := suggestSleepable(suggestRunning(), loadIdleActivity(), cfg.IdleSuspendTimeout(), now)
	if len(names) < suggestMinServices {
		return none
	}
	var mem int64
	for _, c := range suggestStats().Containers {
		if name, ok := strings.CutPrefix(c.Name, "lerd-"); ok && slices.Contains(names, name) {
			mem += c.MemBytes
		}
	}
	if mem < suggestMinMemBytes {
		return none
	}
	return IdleServicesSuggestion{Services: names, MemBytes: mem}
}
