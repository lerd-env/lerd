package cli

import (
	"strings"

	"github.com/geodro/lerd/internal/config"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
)

// A host worker whose command re-enters the container through lerd's shims
// leaves the real process behind when its unit stops: systemd kills the exec
// client on the host while the tool it started keeps running inside, holding
// whatever port it bound. The next start then fails outright for a worker on a
// pinned port, and drifts onto another port for one without, which is worse,
// since the vhost still proxies to the first.

// containerKillFn runs the sweep, a seam so the script can be tested without a
// container.
var containerKillFn = func(container, script string) {
	_ = podman.RunSilent("exec", container, "sh", "-c", script)
}

// shellSingleQuote wraps a value for /bin/sh, so a path or command carrying a
// quote cannot end the string it is embedded in.
func shellSingleQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'"'"'`) + "'"
}

// unquoted strips the quoting a shell has already consumed by the time a
// command is a process. The unit runs `php -r 'work()'`; /proc/<pid>/cmdline
// holds `php -r work()`, so the two only compare after both lose their quotes.
func unquoted(v string) string {
	return strings.NewReplacer("'", "", `"`, "").Replace(v)
}

// containerKillScript matches on both the command and the working directory,
// because sites share a per-version container: two projects running the same
// framework command would otherwise kill each other's worker. It signals the
// process group rather than the process, since the tool a console command
// starts is a child of it and is the one holding the port.
func containerKillScript(command, sitePath string) string {
	// Octal escapes for the quote characters, so this script carries no nested
	// quoting of its own and stays readable in a unit log.
	return `for p in /proc/[0-9]*; do
[ -r "$p/cmdline" ] || continue
c=$(tr "\0" " " < "$p/cmdline" | tr -d "\047\042")
case "$c" in ` + shellSingleQuote(unquoted(command)) + `*) ;; *) continue;; esac
[ "$(readlink "$p/cwd")" = ` + shellSingleQuote(sitePath) + ` ] || continue
g=$(awk "{print \$5}" "$p/stat" 2>/dev/null)
[ -n "$g" ] && kill -TERM "-$g" 2>/dev/null
done`
}

// killWorkerInContainer clears a stopped worker's leftovers from the site's
// runtime. Quiet by design: the worker is being stopped, and a container that
// is not running has nothing to clear.
func killWorkerInContainer(siteName, sitePath, workerName string) {
	if sitePath == "" || workerName == "" {
		return
	}
	site, err := config.FindSite(siteName)
	if err != nil || site == nil {
		return
	}
	fw, ok := config.GetFrameworkForDir(site.Framework, sitePath)
	if !ok || fw == nil {
		return
	}
	w, ok := fw.Workers[workerName]
	if !ok || !w.Host {
		return
	}
	command := resolveWorkerCommand(sitePath, workerName, w)
	if command == "" {
		return
	}
	version, err := phpDet.VersionForDir(sitePath)
	if err != nil {
		return
	}
	container := phpDet.FPMContainerForDir(sitePath, version)
	if container == "" || !podman.ContainerRunningQuiet(container) {
		return
	}
	containerKillFn(container, containerKillScript(command, sitePath))
}
