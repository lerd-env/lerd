package ui

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/geodro/lerd/internal/push"
)

// snapshotRunNotification reports a finished run of the snapshot schedule. Only
// a run that actually took something notifies: a schedule that covers nothing,
// or a tick that was throttled out, has nothing to tell the user about. There is
// deliberately no notification when a run starts, since it is unattended and
// nothing can be done about it while it works.
func snapshotRunNotification(databases, sites int) push.Notification {
	return push.Notification{
		Kind:     "snapshot",
		TitleKey: "notify_snapshot_title",
		Title:    "Snapshots taken",
		BodyKey:  "notify_snapshot_body",
		Body:     snapshotRunBody(databases, sites),
		Params: map[string]string{
			"databases": strconv.Itoa(databases),
			"sites":     strconv.Itoa(sites),
		},
		Tag:     "lerd-snapshot-run",
		URL:     "#system/snapshots",
		Data:    map[string]string{"databases": strconv.Itoa(databases), "sites": strconv.Itoa(sites)},
		Urgency: "low",
		TTL:     3600,
	}
}

func snapshotRunBody(databases, sites int) string {
	dbWord, siteWord := " databases", " sites"
	if databases == 1 {
		dbWord = " database"
	}
	if sites == 1 {
		siteWord = " site"
	}
	return strconv.Itoa(databases) + dbWord + " on " + strconv.Itoa(sites) + siteWord + "."
}

// handleInternalSnapshotNotify lets the watcher, which runs the schedule in its
// own process, emit through the daemon's single notification choke point rather
// than reaching the desktop bus itself and bypassing the category filter.
func handleInternalSnapshotNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var body struct {
		Databases int `json:"databases"`
		Sites     int `json:"sites"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Databases < 1 {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	dispatchNotification(snapshotRunNotification(body.Databases, body.Sites))
	w.WriteHeader(http.StatusNoContent)
}
