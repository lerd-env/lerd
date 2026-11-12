package watcher

import (
	"time"

	"github.com/geodro/lerd/internal/cli"
	"github.com/geodro/lerd/internal/config"
)

// WatchDevServerInstalls restarts a running dev server once its checkout's JS
// packages are installed again, whether the dashboard, a terminal or anything
// else ran the install.
func WatchDevServerInstalls(interval time.Duration) {
	installs := cli.NewDevServerInstalls()
	// The first pass only records each checkout's install, so it runs at once
	// rather than after a tick an install could finish within.
	checkDevServerInstalls(installs)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		checkDevServerInstalls(installs)
	}
}

func checkDevServerInstalls(installs *cli.DevServerInstalls) {
	reg, err := config.LoadSites()
	if err != nil {
		return
	}
	for i := range reg.Sites {
		s := &reg.Sites[i]
		if s.Ignored || s.Paused || s.Framework == "" {
			continue
		}
		if err := installs.Check(s); err != nil {
			logger.Warn("restarting a dev server after an install", "site", s.Name, "err", err)
		}
	}
}
