package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// Events a plugin can hook. The list only grows: a hook for a name this binary
// does not know refuses the plugin, so removing or renaming one breaks plugins.
const (
	EventSiteLinked      = "site.linked"
	EventSiteUnlinked    = "site.unlinked"
	EventSiteSecured     = "site.secured"
	EventSitePHPChanged  = "site.php_changed"
	EventWorktreeAdded   = "worktree.added"
	EventWorktreeRemoved = "worktree.removed"
	EventServiceStarted  = "service.started"
	EventServiceStopped  = "service.stopped"
	EventLerdStarted     = "lerd.started"
	EventLerdStopped     = "lerd.stopped"
)

var events = map[string]bool{
	EventSiteLinked:      true,
	EventSiteUnlinked:    true,
	EventSiteSecured:     true,
	EventSitePHPChanged:  true,
	EventWorktreeAdded:   true,
	EventWorktreeRemoved: true,
	EventServiceStarted:  true,
	EventServiceStopped:  true,
	EventLerdStarted:     true,
	EventLerdStopped:     true,
}

// IsEvent reports whether name is an event plugins can hook.
func IsEvent(name string) bool { return events[name] }

// HookLog is the file in a plugin's data dir that collects its hooks' output.
const HookLog = "hooks.log"

// Emit runs every installed plugin's hook for event, one after another. The
// payload goes in as JSON on stdin with "event" added, and each key also as
// LERD_<KEY>. A hook's output and failure land in its hooks.log; neither ever
// reaches the caller, whose operation already succeeded.
func Emit(event string, payload map[string]string) {
	if !IsEvent(event) {
		panic("plugins: emit of undeclared event " + event)
	}
	list, _ := Load()
	for _, p := range list {
		if run, ok := p.Hooks[event]; ok {
			p.runHook(event, run, payload)
		}
	}
}

// SitePayload is how every surface describes a site to a plugin.
func SitePayload(site *config.Site) map[string]string {
	return map[string]string{
		"site":      site.Name,
		"site_path": site.Path,
		"domain":    site.PrimaryDomain(),
	}
}

// Env turns a payload into the LERD_<KEY> variables a plugin reads it from.
func Env(payload map[string]string) map[string]string {
	env := make(map[string]string, len(payload))
	for k, v := range payload {
		env["LERD_"+strings.ToUpper(k)] = v
	}
	return env
}

func (p *Plugin) runHook(event, run string, payload map[string]string) {
	env := Env(payload)
	env["LERD_EVENT"] = event
	body := map[string]string{"event": event}
	for k, v := range payload {
		body[k] = v
	}
	stdin, _ := json.Marshal(body)

	var out bytes.Buffer
	cmd, err := p.Command(run, SurfaceHook, nil, env)
	if err == nil {
		cmd.Dir = p.Dir
		cmd.Stdin = bytes.NewReader(stdin)
		cmd.Stdout, cmd.Stderr = &out, &out
		err = RunWithin(cmd, hookLimit)
	}

	status := "ok"
	if err != nil {
		status = err.Error()
		Failed(Failure{Plugin: p.Name, Surface: SurfaceHook, What: event, Error: strings.TrimSpace(out.String() + "\n" + status)})
	}
	f, ferr := os.OpenFile(filepath.Join(DataDirFor(p.Name), HookLog), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s: %s\n%s", time.Now().Format(time.RFC3339), event, run, status, out.String())
}
