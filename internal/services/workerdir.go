package services

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// A launchd job has no WorkingDirectory property to read back the way a systemd
// unit does, so on darwin a worker's directory is recorded in the guard script
// lerd generates for it: host-mode workers cd into it, exec-mode workers pass it
// to `podman exec -w`. Reading it back from there is what lets a per-worktree
// unit be told apart from its site's own, and a checkout that is gone be
// recognised as such.
//
// The script is a faithful stand-in for the unit because the two share a
// lifetime: stopping a worker removes the plist and the guard script in the same
// step. So a unit that still exists still has its script, and the orphan case
// this serves — a worker left running by a checkout deleted outside lerd — is
// exactly the case where nothing has removed either.

// WorkerWorkingDirs maps unit name to the directory that unit's worker runs in,
// for every guard script on disk. Both the bare and the ".service" spellings are
// populated, matching what the systemd path returns, so a caller can look a unit
// up in either form. Units with no guard script (container-mode workers, and
// every non-worker unit) are absent, which is the same answer systemd gives for
// a unit that sets no WorkingDirectory.
func WorkerWorkingDirs() map[string]string {
	paths, err := filepath.Glob(filepath.Join(config.RunDir(), "workers", "*.sh"))
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(paths)*2)
	for _, path := range paths {
		script, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		dir := WorkerGuardWorkingDir(string(script))
		if dir == "" {
			continue
		}
		unit := strings.TrimSuffix(filepath.Base(path), ".sh")
		out[unit] = dir
		out[unit+".service"] = dir
	}
	return out
}

// WorkerGuardWorkingDir extracts the directory a guard script runs its worker
// in, or empty when it pins none. Only the two lines the builders emit are
// read: the host guard's own `cd`, and the `-w` on the exec line the guard ends
// with. The reap line names the site path too, but inside a quoted snippet, so
// matching on line shape rather than on the path keeps it out.
//
// Exported so the builders that write these scripts can pin the round trip in
// their own tests, which is what keeps the two shapes from drifting apart.
func WorkerGuardWorkingDir(script string) string {
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "cd '"):
			if dir, ok := singleQuotedPrefix(line[len("cd "):]); ok {
				return dir
			}
		case strings.HasPrefix(line, "exec "):
			if dir := execFlagValue(line, "-w"); dir != "" {
				return dir
			}
		}
	}
	return ""
}

// singleQuotedPrefix reads the leading single-quoted token of s, undoing the
// '"'"' escaping the builders use for a quote inside the value.
func singleQuotedPrefix(s string) (string, bool) {
	if !strings.HasPrefix(s, "'") {
		return "", false
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] != '\'' {
			b.WriteByte(s[i])
			continue
		}
		if strings.HasPrefix(s[i:], `'"'"'`) {
			b.WriteByte('\'')
			i += len(`'"'"'`) - 1
			continue
		}
		return b.String(), true
	}
	return "", false
}

// execFlagValue returns the token following flag in a generated exec line. The
// line is built by joining argv with spaces, so the value is the next field.
func execFlagValue(line, flag string) string {
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == flag && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}
