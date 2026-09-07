package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/hostbin"
	"github.com/geodro/lerd/internal/podman"
)

// ngrok is the one tunnel tool with a published image, so a machine that never
// installed it can still share. The host binary is preferred when it is there,
// because it carries the user's own ngrok configuration; the container carries
// none of it and so cannot authenticate without a token lerd holds.

// ngrokImage is the published image the container route runs.
const ngrokImage = "docker.io/ngrok/ngrok:latest"

// ngrokRunner is how ngrok will be started for this run.
type ngrokRunner struct {
	container bool
	// domain is a reserved ngrok domain the tunnel is pinned to, so the public
	// URL survives the next run. extra carries flags the user asked for that
	// lerd has no opinion on, like a host-header rewrite or a traffic policy.
	domain string
	extra  []string
}

// ngrokRunnerFor picks the route, or explains why neither is usable. A missing
// binary is only fatal once the container is ruled out too.
func ngrokRunnerFor(token string) (ngrokRunner, error) {
	if _, ok := hostbin.Look("ngrok"); ok {
		return ngrokRunner{}, nil
	}
	if token == "" {
		return ngrokRunner{}, fmt.Errorf("ngrok not found — install it from https://ngrok.com/download, or set an auth token with \"lerd share:token\" and lerd runs it as a container instead")
	}
	return ngrokRunner{container: true}, nil
}

// ngrokCmd builds the invocation for whichever route was picked, pointed at the
// local proxy port rather than at nginx.
func (r ngrokRunner) cmd(proxyPort int, token string, headless bool, containerName string) *exec.Cmd {
	if r.container {
		return ngrokContainerCmd(proxyPort, token, headless, containerName, r.tunnelArgs()...)
	}
	return ngrokHostCmd(proxyPort, token, headless, r.tunnelArgs()...)
}

// tunnelArgs is everything the user asked for on top of lerd's own invocation.
func (r ngrokRunner) tunnelArgs() []string {
	var args []string
	if r.domain != "" {
		args = append(args, "--url="+ngrokURL(r.domain))
	}
	return append(args, r.extra...)
}

// ngrokURL spells a reserved domain the way the agent wants it: --url takes a
// full URL, and a bare hostname is what a user reads off the ngrok dashboard.
func ngrokURL(domain string) string {
	if strings.Contains(domain, "://") {
		return domain
	}
	return "https://" + domain
}

// ngrokContainerName is the container a site's tunnel runs as. Deterministic
// because it is how the tunnel is stopped and how a survivor is found again,
// and lerd-prefixed so a running tunnel shows up with lerd's other containers.
func ngrokContainerName(siteName, branch string) string {
	name := "lerd-ngrok-" + siteName
	if branch != "" {
		name += "-" + branch
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// ngrokContainerNames lists the tunnel containers currently running, so a
// survivor of a crash can be found without a pid to go on.
func ngrokContainerNames() []string {
	out, err := podman.Run("ps", "--filter", "name=lerd-ngrok-", "--format", "{{.Names}}")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// removeNgrokContainer stops and removes a tunnel container. Signalling the
// podman client is not enough: conmon is reparented out of the client's process
// tree and cgroup, so a client that was killed rather than asked to stop leaves
// the container serving the site publicly.
func removeNgrokContainer(name string) {
	if name == "" {
		return
	}
	_ = podman.RunSilent("rm", "-f", "-t", "3", name)
}

// ReapOrphanNgrokContainers removes tunnel containers left behind by a previous
// run. Called alongside the pid-based reap on startup, which cannot see these.
func ReapOrphanNgrokContainers() {
	for _, name := range ngrokContainerNames() {
		removeNgrokContainer(name)
	}
}

// ngrokHostCmd runs the installed binary. A configured token is exported rather
// than passed as a flag, so it stays out of the process arguments, and an empty
// one leaves ngrok to read its own config file.
func ngrokHostCmd(proxyPort int, token string, headless bool, extra ...string) *exec.Cmd {
	args := []string{"http", fmt.Sprintf("%d", proxyPort)}
	args = append(args, ngrokLogArgs(headless)...)
	args = append(args, extra...)
	cmd := exec.Command(hostbin.Path("ngrok"), args...)
	cmd.Env = ngrokEnv(token)
	return cmd
}

// ngrokContainerCmd runs the published image against the local proxy. The token
// is exported to podman and forwarded by name: spelling it as -e VAR=VALUE would
// put the secret in the argument list, where any local user can read it off ps.
func ngrokContainerCmd(proxyPort int, token string, headless bool, containerName string, extra ...string) *exec.Cmd {
	return ngrokContainerCmdFor(proxyPort, token, headless, containerName, runtime.GOOS, extra...)
}

// ngrokContainerCmdFor takes the platform so the routing decision can be tested
// on either OS. goos is split out rather than read inline because the two cases
// are not symmetric and only one of them is ever exercised on a given machine.
func ngrokContainerCmdFor(proxyPort int, token string, headless bool, containerName, goos string, extra ...string) *exec.Cmd {
	args := []string{"run", "--rm", "--replace"}
	netArgs, upstream := ngrokContainerUpstream(proxyPort, goos)
	args = append(args, netArgs...)
	mounts, extra := ngrokContainerFiles(extra)
	args = append(args, mounts...)
	args = append(args,
		"--name", containerName,
		"-e", "NGROK_AUTHTOKEN",
		ngrokImage,
		"http", upstream,
	)
	args = append(args, ngrokLogArgs(headless)...)
	args = append(args, extra...)
	cmd := podman.Cmd(args...)
	cmd.Env = ngrokEnv(token)
	return cmd
}

// ngrokContainerUpstream says how the container reaches the proxy, which is a
// host process either way. On Linux --net=host puts the container in the host's
// own network namespace, so the proxy really is on loopback. On macOS the
// container runs inside the podman machine VM, whose loopback is not the host's:
// --net=host there dials the VM, nothing answers, and ngrok reports
// ERR_NGROK_8012. The VM reaches the host over the gateway instead.
func ngrokContainerUpstream(proxyPort int, goos string) (netArgs []string, upstream string) {
	if goos == "darwin" {
		return nil, fmt.Sprintf("http://host.containers.internal:%d", proxyPort)
	}
	return []string{"--net=host"}, fmt.Sprintf("%d", proxyPort)
}

// ngrokLogArgs asks for parseable output when a caller has to scrape the public
// URL rather than show the tool's own screen.
func ngrokLogArgs(headless bool) []string {
	if !headless {
		return nil
	}
	return []string{"--log", "stdout", "--log-format", "json"}
}

// ngrokEnv carries the token to the child without it appearing in argv.
func ngrokEnv(token string) []string {
	env := os.Environ()
	if token == "" {
		return env
	}
	return append(env, "NGROK_AUTHTOKEN="+token)
}

// The flags below are the user's own: lerd passes them through rather than
// growing a lerd flag per ngrok feature. Only the handful lerd itself sets are
// off limits, because a run that reshapes ngrok's log never reports a URL.

// ngrokContainerFileDir is where a host file a flag points at is mounted inside
// the tunnel container.
const ngrokContainerFileDir = "/etc/lerd-ngrok"

// ngrokFileFlags are the flags whose value is a path on the host.
var ngrokFileFlags = map[string]bool{
	"--traffic-policy-file": true,
	"--policy-file":         true,
	"--config":              true,
}

// ngrokOwnedFlags are the flags lerd sets itself.
var ngrokOwnedFlags = map[string]bool{"--log": true, "--log-format": true}

// ngrokDomainFlags are the spellings that pin a reserved domain, which
// "lerd share --domain" already does.
var ngrokDomainFlags = map[string]bool{"--url": true, "--domain": true, "--hostname": true}

// applyNgrokArgs settles the extra ngrok flags for this run. A flag given for
// the run outranks the stored ones without replacing them, the same way the
// token does. Stored flags for a share that turned out to use another tool are
// configuration that simply does not apply; asking for them on that share is a
// mistake worth reporting.
func applyNgrokArgs(tool *shareTool, requested, configured string) error {
	raw := requested
	if raw == "" {
		raw = configured
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if tool.mode != shareModeNgrok {
		if requested == "" {
			return nil
		}
		return fmt.Errorf("--ngrok-args only works with ngrok, and this share uses %s", shareToolCanonicalName(tool))
	}
	args, err := splitNgrokArgs(raw)
	if err != nil {
		return err
	}
	if err := validateNgrokArgs(args, tool.ngrok.domain != ""); err != nil {
		return err
	}
	tool.ngrok.extra = args
	return nil
}

// validateNgrokArgs refuses the flags lerd owns, a second way of saying
// --domain, and a file that is not there. The last one would otherwise surface
// as an opaque ngrok start failure, or on the container route as an empty
// directory podman conjures up in its place.
func validateNgrokArgs(args []string, hasDomain bool) error {
	for i := 0; i < len(args); i++ {
		flag, value, joined := strings.Cut(args[i], "=")
		if ngrokOwnedFlags[flag] {
			return fmt.Errorf("%s is not yours to pass: lerd sets it to read the public URL out of ngrok's log", flag)
		}
		if hasDomain && ngrokDomainFlags[flag] {
			return fmt.Errorf("%s sets the same thing as --domain, pass one or the other", flag)
		}
		if !ngrokFileFlags[flag] {
			continue
		}
		if !joined {
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a file path", flag)
			}
			i++
			value = args[i]
		}
		if _, err := os.Stat(value); err != nil {
			return fmt.Errorf("%s %s: %w", flag, value, err)
		}
	}
	return nil
}

// ngrokContainerFiles mounts the host files the flags point at into the tunnel
// container and repoints the flags at the mounts. A host path means nothing
// inside the container, so without this a policy file is a silent no-op.
func ngrokContainerFiles(extra []string) (mounts, rewritten []string) {
	for i := 0; i < len(extra); i++ {
		flag, value, joined := strings.Cut(extra[i], "=")
		if !ngrokFileFlags[flag] || (!joined && i+1 >= len(extra)) {
			rewritten = append(rewritten, extra[i])
			continue
		}
		if !joined {
			i++
			value = extra[i]
		}
		abs, err := filepath.Abs(value)
		if err != nil {
			abs = value
		}
		inside := ngrokContainerFileDir + "/" + filepath.Base(abs)
		mounts = append(mounts, "-v", abs+":"+inside+":ro")
		if joined {
			rewritten = append(rewritten, flag+"="+inside)
			continue
		}
		rewritten = append(rewritten, flag, inside)
	}
	return mounts, rewritten
}

// splitNgrokArgs splits a flag string the way a shell would, minus expansion.
// Quotes are honoured because a policy file can sit in a path with spaces, and
// that has to survive the round trip through the config file.
func splitNgrokArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	var quote rune
	started := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced %c quote in the ngrok flags: %s", quote, s)
	}
	if started {
		args = append(args, cur.String())
	}
	return args, nil
}
