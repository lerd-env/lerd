package ui

import (
	"errors"
	"net/http"
	"os"

	"github.com/geodro/lerd/internal/applog"
	"github.com/geodro/lerd/internal/config"
)

// handleAppLogsClear deletes the project's application log files (the same set
// the App Logs viewer lists) to reclaim disk, reporting how many files and
// bytes were freed. Loopback-only — it deletes files on the host.
func handleAppLogsClear(w http.ResponseWriter, basePath string, sources []config.FrameworkLogSource) {
	files, bytes, err := clearAppLogs(basePath, sources)
	resp := map[string]any{"ok": err == nil, "files_cleared": files, "bytes_cleared": bytes}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

// clearAppLogs removes every log file matched by the framework's log sources for
// the project at basePath, returning the count and total bytes freed. Each file
// is resolved through the same traversal-guarded discovery the viewer uses, so
// it can only ever touch the declared log globs (e.g. Laravel's
// storage/logs/*.log). A deleted active log is recreated by the app on its next
// write. A per-file delete error is remembered but doesn't stop the sweep, so a
// single locked file can't strand the rest of the reclaim.
func clearAppLogs(basePath string, sources []config.FrameworkLogSource) (filesCleared int, bytesCleared int64, err error) {
	files, derr := applog.DiscoverLogFiles(basePath, sources)
	if derr != nil {
		return 0, 0, derr
	}
	for _, f := range files {
		full := applog.ResolveLogFilePath(basePath, sources, f.Name)
		if full == "" {
			continue
		}
		if rmErr := os.Remove(full); rmErr != nil {
			if !errors.Is(rmErr, os.ErrNotExist) {
				err = rmErr
			}
			continue
		}
		filesCleared++
		bytesCleared += f.Size
	}
	return filesCleared, bytesCleared, err
}
