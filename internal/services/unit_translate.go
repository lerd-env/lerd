package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/hostpath"
	"github.com/geodro/lerd/internal/podman"
)

// keepAlivePolicy mirrors the subset of systemd Restart= values we care
// about; bare KeepAlive=true respawns on clean exit, which is wrong for
// Restart=on-failure (was breaking the tray Quit button).
type keepAlivePolicy int

const (
	keepAliveNever keepAlivePolicy = iota
	keepAliveAlways
	keepAliveOnFailure
)

// --- INI / Quadlet parser ---

// parseSection returns key → []values for a named INI section in content.
func parseSection(content, section string) map[string][]string {
	result := map[string][]string{}
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = line[1:len(line)-1] == section
			continue
		}
		if !inSection {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[key] = append(result[key], val)
		}
	}
	return result
}

// unquoteSystemdValue strips the outer double-quotes that systemd / quadlet
// uses for values that contain spaces or special chars (e.g. Environment=
// "KEY=hello world"). The shell never processes these args, so the quotes
// must be removed before passing to exec.Command.
func unquoteSystemdValue(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`)
	}
	return s
}

// splitSystemdExec tokenises a quadlet Exec= value the way systemd's Quadlet
// generator does on Linux, honouring the double-quoting that shellJoin emits:
// an argument containing whitespace is wrapped in "..." with any inner quote
// escaped as \". macOS has no systemd to parse the unit, so this reverses
// shellJoin before the argv reaches `podman run`. A naive strings.Fields would
// split a quoted `sh -c "<script>"` mid-script and hand the shell a broken,
// unterminated command (the cause of FrankenPHP worker mode failing to boot).
func splitSystemdExec(s string) []string {
	var (
		args     []string
		cur      strings.Builder
		inQuote  bool
		hasToken bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			if c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
				cur.WriteByte(s[i+1])
				i++
			} else if c == '"' {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		case c == '"':
			inQuote = true
			hasToken = true
		case c == ' ' || c == '\t':
			if hasToken {
				args = append(args, cur.String())
				cur.Reset()
				hasToken = false
			}
		default:
			cur.WriteByte(c)
			hasToken = true
		}
	}
	if hasToken {
		args = append(args, cur.String())
	}
	return args
}

// expandSpecifiers replaces Quadlet path specifiers (%h → home dir).
func expandSpecifiers(s string) string {
	home, _ := os.UserHomeDir()
	return strings.ReplaceAll(s, "%h", home)
}

// precreateBindMountDirs creates the host source directory of each bind-mount
// volume so podman run doesn't fail with statfs. A named volume (a bare name
// with no absolute source, e.g. lerd-ssh-agent:/ssh-agent) is skipped: podman
// manages it, and MkdirAll on the bare name would drop a stray relative
// directory into the process working directory.
func precreateBindMountDirs(vols []string) {
	for _, vol := range vols {
		if src, _, _, ok := splitVolume(expandSpecifiers(vol)); ok && isAbsHostPath(src) {
			os.MkdirAll(src, 0755) //nolint:errcheck
		}
	}
}

// isAbsHostPath reports whether a volume source is a host path rather than a
// named volume. filepath.IsAbs alone misses a Windows drive path on the Unix
// build that parses a unit, and a leading-slash path on the Windows one.
func isAbsHostPath(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(p, "/") || hasDriveLetter(p)
}

func hasDriveLetter(p string) bool {
	if len(p) < 3 || p[1] != ':' || (p[2] != '\\' && p[2] != '/') {
		return false
	}
	c := p[0] | 0x20
	return c >= 'a' && c <= 'z'
}

// splitVolume breaks a src:dst[:opts] volume spec apart. A leading Windows drive
// letter (C:\site or C:/site) owns its colon, so it stays part of src.
func splitVolume(vol string) (src, dst, opts string, ok bool) {
	src, rest, ok := cutVolumeField(vol)
	if !ok {
		return "", "", "", false
	}
	dst, opts, _ = cutVolumeField(rest)
	return src, dst, opts, true
}

// cutVolumeField returns the text up to the next separator colon and what
// follows it. A leading drive letter keeps its own colon. ok is false when no
// separator was found, in which case rest is empty.
func cutVolumeField(s string) (field, rest string, ok bool) {
	skip := 0
	if hasDriveLetter(s) {
		skip = 2
	}
	i := strings.IndexByte(s[skip:], ':')
	if i < 0 {
		return s, "", false
	}
	return s[:skip+i], s[skip+i+1:], true
}

// stripSELinuxVolOpts removes SELinux relabelling flags (:z, :Z) from a
// volume mount spec. With Podman Machine the source path is a virtiofs or
// 9p mount and SELinux relabelling is unsupported; passing :z causes the
// container to fail to start.
func stripSELinuxVolOpts(vol string) string {
	src, dst, opts, ok := splitVolume(vol)
	if !ok || opts == "" {
		return vol
	}
	var filtered []string
	for _, o := range strings.Split(opts, ",") {
		if o != "z" && o != "Z" {
			filtered = append(filtered, o)
		}
	}
	if len(filtered) == 0 {
		return src + ":" + dst
	}
	return src + ":" + dst + ":" + strings.Join(filtered, ",")
}

// stripPrivilegedIPBind removes the host-IP prefix from a PublishPort value
// when the host port is privileged (< 1024). gvproxy on macOS rejects
// explicit IP binds for privileged ports with "bind: permission denied".
// Handles both v4 ("127.0.0.1:80:80" → "80:80") and bracketed v6
// ("[::1]:443:443" → "443:443"). Non-privileged ports keep their bind so
// LAN restriction is preserved.
func stripPrivilegedIPBind(port string) string {
	var rest string
	if strings.HasPrefix(port, "[") {
		end := strings.Index(port, "]")
		if end < 0 || end+1 >= len(port) || port[end+1] != ':' {
			return port
		}
		rest = port[end+2:]
	} else {
		parts := strings.SplitN(port, ":", 3)
		if len(parts) != 3 {
			return port
		}
		rest = parts[1] + ":" + parts[2]
	}
	hostPortStr := strings.SplitN(strings.SplitN(rest, ":", 2)[0], "/", 2)[0]
	n := 0
	for _, c := range hostPortStr {
		if c < '0' || c > '9' {
			return port
		}
		n = n*10 + int(c-'0')
	}
	if n > 0 && n < 1024 {
		return rest
	}
	return port
}

// stripIPv6PublishPorts normalises bracketed IPv6 PublishPort= lines for gvproxy,
// which cannot bind both an IPv4 and an IPv6 address on the same port at once.
//
// Loopback IPv6 lines ("[::1]:3306:3306") are dropped: PairIPv6Binds always pairs
// them with an IPv4 "127.0.0.1:" line that survives, so the published port lives on.
//
// Bind-all IPv6 lines ("[::]:80:80") have no IPv4 partner — PairIPv6Binds collapses a
// bare/0.0.0.0 bind into the "[::]:" form only — so they are rewritten to "0.0.0.0:"
// rather than dropped. Dropping them left LAN-exposed containers (lan.exposed: true,
// where every publish ends up as "[::]:") with no published ports at all.
func stripIPv6PublishPorts(content string) string {
	lines := strings.Split(content, "\n")
	out := lines[:0]
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		val := strings.TrimPrefix(trimmed, "PublishPort=")
		if val == trimmed || !strings.HasPrefix(val, "[") {
			out = append(out, l)
			continue
		}
		if rest, ok := strings.CutPrefix(val, "[::]:"); ok {
			out = append(out, "PublishPort=0.0.0.0:"+rest)
			continue
		}
		// Loopback (or any other bracketed) IPv6 line: drop it; its IPv4 partner stays.
	}
	return strings.Join(out, "\n")
}

// stopTimeoutFlag is podman run's graceful-stop flag. Named once because the
// window reaches this file under two spellings and has to leave under one.
const stopTimeoutFlag = "--stop-timeout"

// quadletStopTimeout reads the graceful-stop window the quadlet declares, so
// macOS gives a container the same grace the Linux unit would. Falls back to
// the default for a quadlet written before the key existed.
func quadletStopTimeout(c map[string][]string) int {
	if v := c["StopTimeout"]; len(v) > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(v[0])); err == nil && n > 0 {
			return n
		}
	}
	// The generator writes the window as PodmanArgs= instead whenever it could
	// not confirm podman is 5.0 or newer, which includes every time the version
	// probe fails to run. That happens under launchd's restricted PATH, so this
	// spelling is the reachable one here rather than the exotic one.
	for _, extra := range c["PodmanArgs"] {
		for _, field := range strings.Fields(extra) {
			raw, ok := strings.CutPrefix(field, stopTimeoutFlag+"=")
			if !ok {
				continue
			}
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				return n
			}
		}
	}
	return config.DefaultStopTimeout
}

// containerToPodmanArgs builds a podman run argument list from a parsed [Container] section.
// On macOS we run detached (-d) so that launchctl bootstrap sees an immediate
// exit 0 (success); podman's own --restart=always policy handles crash recovery.
func containerToPodmanArgs(c map[string][]string) ([]string, error) {
	args := []string{podman.PodmanBin(), "run", "-d", "--restart=always"}

	if names := c["ContainerName"]; len(names) > 0 {
		// --replace removes any stale container with this name before starting.
		// --stop-timeout keeps a restart quick for images that exit promptly,
		// and honours the longer grace a service declared for itself rather
		// than killing it mid-write on the macOS path only.
		args = append(args, "--name", names[0], "--replace",
			fmt.Sprintf("%s=%d", stopTimeoutFlag, quadletStopTimeout(c)))
	}
	for _, net := range c["Network"] {
		args = append(args, "--network", net)
	}
	for _, port := range c["PublishPort"] {
		args = append(args, "-p", stripPrivilegedIPBind(port))
	}
	for _, vol := range c["Volume"] {
		args = append(args, "-v", stripSELinuxVolOpts(expandSpecifiers(vol)))
	}
	for _, env := range c["Environment"] {
		args = append(args, "-e", unquoteSystemdValue(env))
	}
	if userns := c["UserNS"]; len(userns) > 0 {
		args = append(args, "--userns", userns[0])
	}
	if hns := c["HostName"]; len(hns) > 0 {
		args = append(args, "--hostname", hns[0])
	}
	if dirs := c["WorkingDir"]; len(dirs) > 0 {
		args = append(args, "--workdir", expandSpecifiers(dirs[0]))
	}
	for _, extra := range c["PodmanArgs"] {
		for _, field := range strings.Fields(expandSpecifiers(extra)) {
			// The stop window was already emitted above from whichever spelling
			// declared it. Letting the PodmanArgs copy through too would put the
			// flag on the command line twice and leave which one applies resting
			// on argument order.
			if strings.HasPrefix(field, stopTimeoutFlag+"=") {
				continue
			}
			args = append(args, field)
		}
	}

	images := c["Image"]
	if len(images) == 0 {
		return nil, fmt.Errorf("no Image= found in [Container] section")
	}
	args = append(args, images[0])

	for _, cmd := range c["Exec"] {
		args = append(args, splitSystemdExec(cmd)...)
	}

	return args, nil
}

// --- Service unit files ---

// parseServiceUnit parses a systemd-format service unit and returns the argv
// and keepAlive policy for the launchd plist.
//
// Binary resolution rules for args[0]:
//   - Absolute path that exists → use as-is.
//   - Absolute path that doesn't exist → substitute the running lerd binary
//     (handles Homebrew → ~/.local/bin migration).
//   - Bare command name (no '/') → resolve via PATH; if not found, substitute
//     the running lerd binary (should not normally happen).
func parseServiceUnit(name, content string) (args []string, keepAlive keepAlivePolicy, err error) {
	svc := parseSection(content, "Service")
	execStarts := svc["ExecStart"]
	if len(execStarts) == 0 {
		return nil, keepAliveNever, fmt.Errorf("no ExecStart= found in service unit %s", name)
	}
	args = SplitExecStart(expandSpecifiers(execStarts[0]))
	if len(args) == 0 {
		return nil, keepAliveNever, fmt.Errorf("empty ExecStart in service unit %s", name)
	}

	// Resolve args[0] to an absolute path suitable for a launchd plist.
	if filepath.IsAbs(args[0]) {
		// Absolute path: substitute if missing (e.g. old Homebrew install).
		if _, statErr := os.Stat(args[0]); statErr != nil {
			args[0] = missingBinaryFallback(args[0])
		}
	} else {
		// Bare command (e.g. "podman"): resolve via PATH first, then well-known
		// Homebrew locations. Never fall back to the lerd binary — if the command
		// cannot be found, return an error so the caller can surface a clear message.
		resolved := ""
		if p, lookErr := exec.LookPath(args[0]); lookErr == nil {
			resolved = p
		} else {
			for _, dir := range fallbackBinDirs {
				candidate := filepath.Join(dir, args[0])
				if _, statErr := os.Stat(candidate); statErr == nil {
					resolved = candidate
					break
				}
			}
		}
		if resolved == "" {
			return nil, keepAliveNever, fmt.Errorf("command %q in ExecStart of %s not found; use an absolute path", args[0], name)
		}
		args[0] = resolved
	}

	// Map Restart= to a launchd policy. `Restart=on-failure` translates to
	// KeepAlive: SuccessfulExit=false so a clean exit (e.g. tray Quit) is
	// honoured; only crashes or non-zero exits trigger a respawn.
	restart := ""
	if restarts := svc["Restart"]; len(restarts) > 0 {
		restart = restarts[0]
	}
	switch restart {
	case "always":
		keepAlive = keepAliveAlways
	case "on-failure":
		keepAlive = keepAliveOnFailure
	default:
		keepAlive = keepAliveNever
	}
	return args, keepAlive, nil
}

// fallbackBinDirs are searched for a bare ExecStart command PATH cannot find.
// A platform sets it from an init() when its service manager runs with a
// restricted PATH.
var fallbackBinDirs []string

// podmanStartSem limits concurrent `podman run` executions to avoid
// overwhelming the Podman Machine SSH connection with parallel requests.
var podmanStartSem = make(chan struct{}, 4)

// runPodmanWithError invokes the podman command and surfaces stderr in the
// returned error. The launcher process detaches once `podman run -d` accepts
// the request, so a non-nil error here means podman itself rejected the run
// (image missing, port collision, machine down) — exactly the cases that
// were previously masked under //nolint:errcheck and made the heal loop
// report success on a unit that never started.
func runPodmanWithError(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, trimmed)
	}
	return nil
}

// mapContainerPaths rewrites the container side of a parsed [Container]
// section for a host whose paths do not exist inside the VM. A volume keeps its
// host source, which podman resolves itself, but its destination and the working
// directory must be paths the Linux guest can hold, so a Windows drive path
// becomes its mount under hostpath.DriveRoot. Specifiers such as %h expand first,
// since the mapping only recognises a Windows path once the home dir is in it.
func mapContainerPaths(c map[string][]string) {
	for i, vol := range c["Volume"] {
		src, dst, opts, ok := splitVolume(expandSpecifiers(vol))
		if !ok {
			continue
		}
		out := src + ":" + hostpath.ToVM(dst)
		if opts != "" {
			out += ":" + opts
		}
		c["Volume"][i] = out
	}
	for i, dir := range c["WorkingDir"] {
		c["WorkingDir"][i] = hostpath.ToVM(expandSpecifiers(dir))
	}
}

// rebaseLerdDirs points the Linux spelling of lerd's directories that embedded
// units use (%h/.local/share/lerd, %h/.config/lerd) at where this host keeps
// them. A host whose data dir is the XDG default is unaffected; one that keeps
// it elsewhere (Windows) would otherwise mount paths that do not exist.
func rebaseLerdDirs(content string) string {
	return strings.NewReplacer(
		"%h/.local/share/lerd", config.DataDir(),
		"%h/.config/lerd", config.ConfigDir(),
	).Replace(content)
}
