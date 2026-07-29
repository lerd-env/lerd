package cli

import (
	"fmt"
	"os"
	"os/exec"
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
		return ngrokContainerCmd(proxyPort, token, headless, containerName)
	}
	return ngrokHostCmd(proxyPort, token, headless)
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
func ngrokHostCmd(proxyPort int, token string, headless bool) *exec.Cmd {
	args := []string{"http", fmt.Sprintf("%d", proxyPort)}
	args = append(args, ngrokLogArgs(headless)...)
	cmd := exec.Command(hostbin.Path("ngrok"), args...)
	cmd.Env = ngrokEnv(token)
	return cmd
}

// ngrokContainerCmd runs the published image on the host network, so it reaches
// the proxy on loopback the same way the binary would. The token is exported to
// podman and forwarded by name: spelling it as -e VAR=VALUE would put the secret
// in the argument list, where any local user can read it off ps.
func ngrokContainerCmd(proxyPort int, token string, headless bool, containerName string) *exec.Cmd {
	args := []string{
		"run", "--rm", "--replace", "--net=host",
		"--name", containerName,
		"-e", "NGROK_AUTHTOKEN",
		ngrokImage,
		"http", fmt.Sprintf("%d", proxyPort),
	}
	args = append(args, ngrokLogArgs(headless)...)
	cmd := podman.Cmd(args...)
	cmd.Env = ngrokEnv(token)
	return cmd
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
