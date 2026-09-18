package podman

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed quadlets
var quadletFS embed.FS

// GetQuadletTemplate returns the content of a named quadlet template file.
func GetQuadletTemplate(name string) (string, error) {
	data, err := quadletFS.ReadFile("quadlets/" + name)
	if err != nil {
		return "", fmt.Errorf("quadlet template %q not found: %w", name, err)
	}
	return string(data), nil
}

// ApplyImage replaces the Image= line in quadlet content with the given image.
// If content contains no Image= line it is returned unchanged.
func ApplyImage(content, image string) string {
	if image == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Image=") {
			lines[i] = "Image=" + image
			return strings.Join(lines, "\n")
		}
	}
	return content
}

// CurrentImage returns the value of the Image= line in quadlet content,
// or "" if no such line exists.
func CurrentImage(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Image=") {
			return strings.TrimPrefix(trimmed, "Image=")
		}
	}
	return ""
}

// SetPrimaryHostPort rewrites the host (published) side of the first port spec
// in ports to hostPort, preserving the container port and any /proto suffix.
// The primary mapping is index 0 (the preset's own port; extra ports follow),
// so this moves where the service is reachable on the host without touching the
// container-internal port. A spec may be "host:container" or "ip:host:container".
// Returns ports unchanged when empty, hostPort is non-positive, or the first
// spec is unrecognised. Pure; operates on the pre-render svc.Ports form (before
// BindForLAN/PairIPv6Binds add the loopback/IPv6 prefixes).
func SetPrimaryHostPort(ports []string, hostPort int) []string {
	if len(ports) == 0 || hostPort <= 0 {
		return ports
	}
	spec := ports[0]
	proto := ""
	if slash := strings.IndexByte(spec, '/'); slash >= 0 {
		proto, spec = spec[slash:], spec[:slash]
	}
	segs := strings.Split(spec, ":")
	switch len(segs) {
	case 2: // host:container
		segs[0] = strconv.Itoa(hostPort)
	case 3: // ip:host:container
		segs[1] = strconv.Itoa(hostPort)
	default:
		return ports
	}
	out := append([]string(nil), ports...)
	out[0] = strings.Join(segs, ":") + proto
	return out
}

// PrimaryHostPort returns the host (published) port of the first port spec in
// ports — the read side of SetPrimaryHostPort. A spec may be "host:container"
// or "ip:host:container", optionally with a /proto suffix. Returns 0 when ports
// is empty or the first spec has no parseable host port (e.g. a bare container
// port or an empty host segment for podman auto-assignment).
func PrimaryHostPort(ports []string) int {
	if len(ports) == 0 {
		return 0
	}
	spec := ports[0]
	if slash := strings.IndexByte(spec, '/'); slash >= 0 {
		spec = spec[:slash]
	}
	segs := strings.Split(spec, ":")
	var host string
	switch len(segs) {
	case 2: // host:container
		host = segs[0]
	case 3: // ip:host:container
		host = segs[1]
	default:
		return 0
	}
	n, err := strconv.Atoi(host)
	if err != nil {
		return 0
	}
	return n
}

// ContainerPort returns the container (internal) port of a port spec — the last
// numeric segment of "host:container" or "ip:host:container", ignoring any
// /proto suffix. Returns 0 for a bare host port or an unparseable spec.
func ContainerPort(spec string) int {
	if slash := strings.IndexByte(spec, '/'); slash >= 0 {
		spec = spec[:slash]
	}
	segs := strings.Split(spec, ":")
	if len(segs) < 2 {
		return 0
	}
	n, err := strconv.Atoi(segs[len(segs)-1])
	if err != nil {
		return 0
	}
	return n
}

// SetHostPortForContainerPort rewrites the host (published) side of whichever
// spec in ports maps to containerPort, leaving the container-internal port and
// every other mapping untouched. It generalises SetPrimaryHostPort past the
// index-0 primary so a service's secondary published port (mailpit's 8025 UI,
// rustfs' 9001 console) can be remapped too. Returns ports unchanged when empty,
// hostPort is non-positive, or no spec maps to containerPort. Pure.
func SetHostPortForContainerPort(ports []string, containerPort, hostPort int) []string {
	if len(ports) == 0 || hostPort <= 0 || containerPort <= 0 {
		return ports
	}
	for i, spec := range ports {
		if ContainerPort(spec) != containerPort {
			continue
		}
		body, proto := spec, ""
		if slash := strings.IndexByte(spec, '/'); slash >= 0 {
			body, proto = spec[:slash], spec[slash:]
		}
		segs := strings.Split(body, ":")
		switch len(segs) {
		case 2: // host:container
			segs[0] = strconv.Itoa(hostPort)
		case 3: // ip:host:container
			segs[1] = strconv.Itoa(hostPort)
		default:
			return ports
		}
		out := append([]string(nil), ports...)
		out[i] = strings.Join(segs, ":") + proto
		return out
	}
	return ports
}

// ApplyNginxPorts rewrites the host side of nginx's PublishPort lines to the
// configured ports, keeping the container side on 80/443. A non-positive port
// leaves its mapping alone, so an unset half keeps the bundled default.
func ApplyNginxPorts(content string, httpPort, httpsPort int) string {
	want := map[string]int{"80": httpPort, "443": httpsPort}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "PublishPort=") {
			continue
		}
		spec := strings.TrimPrefix(trimmed, "PublishPort=")
		proto := ""
		if slash := strings.IndexByte(spec, '/'); slash >= 0 {
			proto, spec = spec[slash:], spec[:slash]
		}
		segs := strings.Split(spec, ":")
		if len(segs) < 2 {
			continue
		}
		host := len(segs) - 2
		port, ok := want[segs[len(segs)-1]]
		if !ok || port <= 0 {
			continue
		}
		segs[host] = strconv.Itoa(port)
		lines[i] = "PublishPort=" + strings.Join(segs, ":") + proto
	}
	return strings.Join(lines, "\n")
}

// ApplyExtraPorts adds extra PublishPort lines to the [Container] section.
// Quadlet ignores the key in any other section, so they can't just be appended.
func ApplyExtraPorts(content string, extraPorts []string) string {
	if len(extraPorts) == 0 {
		return content
	}
	lines := make([]string, 0, len(extraPorts))
	for _, p := range extraPorts {
		lines = append(lines, "PublishPort="+p)
	}
	return insertAfterImage(content, lines...)
}

// insertAfterImage places lines right after the Image= line, which always sits
// in [Container]. Content without an Image= line is returned unchanged.
func insertAfterImage(content string, extra ...string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Image=") {
			out := make([]string, 0, len(lines)+len(extra))
			out = append(out, lines[:i+1]...)
			out = append(out, extra...)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return content
}

// StripInstallSection removes the [Install] section from a quadlet's content
// when autostartDisabled is true, and returns the input unchanged when false.
//
// Quadlets are special: a `[Install] WantedBy=default.target` clause causes
// the podman-system-generator to create a symlink in
// `/run/user/$UID/systemd/generator/default.target.wants/` on every
// daemon-reload, which makes the unit auto-start at login regardless of
// `systemctl --user enable/disable` (those don't apply to generator units).
// The only way to actually stop a quadlet from auto-starting is to drop the
// [Install] section from the source .container file before the generator
// sees it. WriteQuadletDiff calls this centrally so every code path that
// writes a quadlet (install, services, MCP server, custom-service generator)
// honours the global autostart setting without each having to remember.
func StripInstallSection(content string, autostartDisabled bool) string {
	if !autostartDisabled {
		return content
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inInstall := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inInstall = trimmed == "[Install]"
			if inInstall {
				continue
			}
		}
		if inInstall {
			continue
		}
		out = append(out, line)
	}
	// Trim a trailing run of blank lines that would otherwise be left
	// behind when [Install] was the last section in the file.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	out = append(out, "")
	return strings.Join(out, "\n")
}

// InjectPodmanArgs adds `PodmanArgs=<arg>` to the [Container] section.
// Idempotent: if any PodmanArgs= line already carries the same arg we
// return unchanged so the quadlet diff doesn't oscillate across writes.
func InjectPodmanArgs(content, arg string) string {
	if arg == "" {
		return content
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "PodmanArgs=") {
			continue
		}
		for _, f := range strings.Fields(strings.TrimPrefix(trimmed, "PodmanArgs=")) {
			if f == arg {
				return content
			}
		}
	}
	return insertAfterImage(content, "PodmanArgs="+arg)
}

// InjectExtraVolumes adds Volume= lines for paths that are not already covered
// by the %h:%h mount. Each path is bind-mounted read-write at the same location
// inside the container. Existing Volume= lines for the same host path are not
// duplicated. Paths that cannot be bind-mounted are skipped: this is the one
// place that formats a Volume=path:path line, so the guard belongs here (#884).
func InjectExtraVolumes(content string, paths []string) string {
	if len(paths) == 0 {
		return content
	}
	var extra []string
	for _, p := range paths {
		if !bindMountable(p) {
			continue
		}
		// Check if this path is already mounted (with any flags).
		prefix := fmt.Sprintf("Volume=%s:%s:", p, p)
		if strings.Contains(content, prefix) {
			continue
		}
		extra = append(extra, fmt.Sprintf("Volume=%s:%s:rw", p, p))
	}
	if len(extra) == 0 {
		return content
	}
	// Insert after the Volume=%h:%h line (matches both :rw and :ro).
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Volume=%h:%h:") {
			out := make([]string, 0, len(lines)+len(extra))
			out = append(out, lines[:i+1]...)
			out = append(out, extra...)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return content
}

// OCIRuntime returns the name of the OCI runtime podman is currently configured to use.
func OCIRuntime() string {
	out, err := execCommand(PodmanBin(), "info", "--format", "{{.Host.OCIRuntime.Name}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// BindForLAN flips every PublishPort= between the loopback and LAN form on
// both stacks in lockstep: 127.0.0.1 ↔ bare and [::1] ↔ [::]. lerd-dns
// (:5300) is pinned on 127.0.0.1 in the embed because LAN access routes
// via the userspace forwarder, so its lines are preserved as-is.
func BindForLAN(content string, lanExposed bool) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "PublishPort=") {
			continue
		}
		// Preserve lerd-dns (pinned to 127.0.0.1 in the embed because LAN
		// DNS is routed via the userspace forwarder, not the publish).
		if strings.Contains(trimmed, ":5300:5300") {
			continue
		}
		value := strings.TrimPrefix(trimmed, "PublishPort=")

		if lanExposed {
			if rest, ok := strings.CutPrefix(value, "127.0.0.1:"); ok {
				lines[i] = "PublishPort=" + rest
			} else if rest, ok := strings.CutPrefix(value, "[::1]:"); ok {
				lines[i] = "PublishPort=[::]:" + rest
			}
			continue
		}

		if rest, ok := strings.CutPrefix(value, "[::]:"); ok {
			lines[i] = "PublishPort=[::1]:" + rest
			continue
		}
		if strings.HasPrefix(value, "[") {
			continue
		}
		firstSeg := strings.SplitN(value, ":", 2)[0]
		if strings.ContainsRune(firstSeg, '.') {
			continue
		}
		lines[i] = "PublishPort=127.0.0.1:" + value
	}
	return strings.Join(lines, "\n")
}

// PairIPv6Binds normalises PublishPort lines for dual-stack reach:
// 127.0.0.1:X gets paired with [::1]:X, bare/0.0.0.0:X is rewritten
// to [::]:X. Idempotent; skipped when Network= is absent (pasta path).
func PairIPv6Binds(content string) string {
	hasNetwork := false
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Network=") {
			hasNetwork = true
			break
		}
	}
	if !hasNetwork {
		return content
	}
	lines := strings.Split(content, "\n")

	v4LoopbackPortSpecs := map[string]bool{}
	v6PortSpecs := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "PublishPort=") {
			continue
		}
		value := strings.TrimPrefix(trimmed, "PublishPort=")
		if strings.HasPrefix(value, "127.0.0.1:") {
			v4LoopbackPortSpecs[strings.TrimPrefix(value, "127.0.0.1:")] = true
			continue
		}
		if !strings.HasPrefix(value, "[") {
			continue
		}
		end := strings.Index(value, "]")
		if end < 0 || end+1 >= len(value) || value[end+1] != ':' {
			continue
		}
		v6PortSpecs[value[end+2:]] = true
	}

	out := make([]string, 0, len(lines)*2)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "PublishPort=") {
			out = append(out, line)
			continue
		}
		value := strings.TrimPrefix(trimmed, "PublishPort=")
		if strings.HasPrefix(value, "[") {
			out = append(out, line)
			if rest, ok := strings.CutPrefix(value, "[::1]:"); ok && !v4LoopbackPortSpecs[rest] {
				out = append(out, "PublishPort=127.0.0.1:"+rest)
				v4LoopbackPortSpecs[rest] = true
			}
			continue
		}

		firstSeg := strings.SplitN(value, ":", 2)[0]
		switch {
		case !strings.ContainsRune(firstSeg, '.'), firstSeg == "0.0.0.0":
			portSpec := strings.TrimPrefix(value, "0.0.0.0:")
			if v6PortSpecs[portSpec] {
				continue
			}
			v6PortSpecs[portSpec] = true
			out = append(out, "PublishPort=[::]:"+portSpec)
		case firstSeg == "127.0.0.1":
			out = append(out, line)
			portSpec := strings.TrimPrefix(value, "127.0.0.1:")
			if v6PortSpecs[portSpec] {
				continue
			}
			v6PortSpecs[portSpec] = true
			out = append(out, "PublishPort=[::1]:"+portSpec)
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// hostIPv6LoopbackFn is swapped in tests to stage a host without ::1.
var hostIPv6LoopbackFn = HostHasIPv6Loopback

// applyIPv6BindPolicy pairs PublishPort lines across both stacks, or reduces
// them to IPv4 on a host that has no ::1 (VPN clients and ipv6.disable=1 both
// take it away). rootlessport treats a failed v6 bind as fatal, so a stale
// [::1] line stops the unit from starting at all instead of degrading.
func applyIPv6BindPolicy(content string) string {
	if hostIPv6LoopbackFn() {
		return PairIPv6Binds(content)
	}
	return StripIPv6Binds(content)
}

// StripIPv6Binds is the inverse of PairIPv6Binds: every IPv6 PublishPort line
// is dropped when its IPv4 twin is already there, and rewritten to that twin
// when it is not, so the unit keeps serving on IPv4 alone. Operator-set
// addresses are left verbatim. Idempotent.
func StripIPv6Binds(content string) string {
	lines := strings.Split(content, "\n")

	v4Loopback := map[string]bool{}
	v4Any := map[string]bool{}
	for _, line := range lines {
		value, ok := publishPortValue(line)
		if !ok || strings.HasPrefix(value, "[") {
			continue
		}
		if rest, isLoopback := strings.CutPrefix(value, "127.0.0.1:"); isLoopback {
			v4Loopback[rest] = true
			continue
		}
		v4Any[strings.TrimPrefix(value, "0.0.0.0:")] = true
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		value, ok := publishPortValue(line)
		if !ok {
			out = append(out, line)
			continue
		}
		if rest, isV6Loopback := strings.CutPrefix(value, "[::1]:"); isV6Loopback {
			if !v4Loopback[rest] {
				v4Loopback[rest] = true
				out = append(out, "PublishPort=127.0.0.1:"+rest)
			}
			continue
		}
		if rest, isV6Any := strings.CutPrefix(value, "[::]:"); isV6Any {
			if !v4Any[rest] {
				v4Any[rest] = true
				out = append(out, "PublishPort="+rest)
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// publishPortValue returns the address:port body of a PublishPort= line.
func publishPortValue(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "PublishPort=") {
		return "", false
	}
	return strings.TrimPrefix(trimmed, "PublishPort="), true
}
