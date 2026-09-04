package cli

import (
	"os"
	"strings"
	"testing"
)

// The host binary is preferred whenever it is there: it carries the user's own
// ngrok configuration, which a container cannot see.
func TestNgrokRunnerPrefersTheHostBinary(t *testing.T) {
	dir := t.TempDir()
	writeFakeExe(t, dir+"/ngrok")
	t.Setenv("PATH", dir)

	runner, err := ngrokRunnerFor("token-abc")
	if err != nil {
		t.Fatalf("ngrokRunnerFor() error: %v", err)
	}
	if runner.container {
		t.Fatal("host ngrok is installed, the container should not have been chosen")
	}
}

// Without the binary the published image stands in, so a machine that never
// installed ngrok can still share.
func TestNgrokRunnerFallsBackToTheContainer(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	runner, err := ngrokRunnerFor("token-abc")
	if err != nil {
		t.Fatalf("ngrokRunnerFor() error: %v", err)
	}
	if !runner.container {
		t.Fatal("no host ngrok, the container should have been chosen")
	}
}

// A container has no ngrok configuration of its own, so without a token it
// would start and immediately fail. Say so while it can still be acted on.
func TestNgrokRunnerNeedsATokenForTheContainer(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := ngrokRunnerFor("")
	if err == nil {
		t.Fatal("a container run without a token should have been refused")
	}
	if !strings.Contains(err.Error(), "share:token") {
		t.Errorf("the error should point at the command that fixes it, got: %v", err)
	}
}

// The token is an argument to lerd, never to podman: an -e VAR=VALUE would put
// it in the process arguments, where any local user can read it off ps.
func TestNgrokContainerKeepsTheTokenOutOfTheArguments(t *testing.T) {
	cmd := ngrokContainerCmd(5180, "super-secret-token", false, "lerd-ngrok-t")

	for _, arg := range cmd.Args {
		if strings.Contains(arg, "super-secret-token") {
			t.Fatalf("the token reached the command line: %v", cmd.Args)
		}
	}
	var carried bool
	for _, env := range cmd.Env {
		if env == "NGROK_AUTHTOKEN=super-secret-token" {
			carried = true
		}
	}
	if !carried {
		t.Errorf("the token was not passed through the environment: %v", cmd.Env)
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-e", "NGROK_AUTHTOKEN", ngrokImage, "http"} {
		if !strings.Contains(joined, want) {
			t.Errorf("container command missing %q: %s", want, joined)
		}
	}
}

// On macOS the container runs inside the podman machine VM, whose loopback is
// not the host's. --net=host there points ngrok at the VM, where nothing is
// listening, and every request comes back ERR_NGROK_8012.
func TestNgrokContainerReachesTheProxyOverTheHostGatewayOnMacOS(t *testing.T) {
	joined := strings.Join(ngrokContainerCmdFor(65483, "tok", false, "lerd-ngrok-t", "darwin").Args, " ")

	if strings.Contains(joined, "--net=host") {
		t.Errorf("the podman machine VM's loopback is not the host's: %s", joined)
	}
	if !strings.Contains(joined, "http http://host.containers.internal:65483") {
		t.Errorf("container is not pointed at the host gateway: %s", joined)
	}
}

// On Linux the container shares the host's own network namespace, so the proxy
// really is on loopback and the bare port is what ngrok should be given.
func TestNgrokContainerUsesHostNetworkingOnLinux(t *testing.T) {
	joined := strings.Join(ngrokContainerCmdFor(65483, "tok", false, "lerd-ngrok-t", "linux").Args, " ")

	if !strings.Contains(joined, "--net=host") {
		t.Errorf("linux should share the host network namespace: %s", joined)
	}
	if !strings.Contains(joined, "http 65483") {
		t.Errorf("linux should dial the proxy on loopback: %s", joined)
	}
}

// Headless runs are scraped for the public URL, so the tool has to be asked for
// parseable output on both routes.
func TestNgrokHeadlessAsksForJSONLogsOnBothRoutes(t *testing.T) {
	container := strings.Join(ngrokContainerCmd(5180, "tok", true, "lerd-ngrok-t").Args, " ")
	if !strings.Contains(container, "--log-format json") {
		t.Errorf("container headless run is not parseable: %s", container)
	}
	host := strings.Join(ngrokHostCmd(5180, "tok", true).Args, " ")
	if !strings.Contains(host, "--log-format json") {
		t.Errorf("host headless run is not parseable: %s", host)
	}
}

// A configured token authenticates the host binary too, so a machine that has
// ngrok installed but never ran "ngrok config add-authtoken" still works.
func TestNgrokHostCarriesTheTokenWhenOneIsSet(t *testing.T) {
	cmd := ngrokHostCmd(5180, "super-secret-token", false)

	for _, arg := range cmd.Args {
		if strings.Contains(arg, "super-secret-token") {
			t.Fatalf("the token reached the command line: %v", cmd.Args)
		}
	}
	var carried bool
	for _, env := range cmd.Env {
		if env == "NGROK_AUTHTOKEN=super-secret-token" {
			carried = true
		}
	}
	if !carried {
		t.Errorf("the token was not passed through the environment: %v", cmd.Env)
	}
}

// Without a token the host binary falls back to its own config file, so nothing
// should be added to the environment.
func TestNgrokHostWithoutATokenLeavesTheEnvironmentAlone(t *testing.T) {
	cmd := ngrokHostCmd(5180, "", false)
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "NGROK_AUTHTOKEN=") {
			t.Errorf("an empty token was still exported: %v", env)
		}
	}
}

func writeFakeExe(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// A container outlives the podman client that started it: conmon is reparented
// away and sits in its own cgroup, so neither a SIGKILL of the client nor a
// control-group stop of lerd-ui reaches it. It has to be addressable by name.
func TestNgrokContainerIsNamedForCleanupAndTheResourcesView(t *testing.T) {
	cmd := ngrokContainerCmd(5180, "tok", false, "lerd-ngrok-acme")
	joined := strings.Join(cmd.Args, " ")

	if !strings.Contains(joined, "--name lerd-ngrok-acme") {
		t.Errorf("container is not addressable by name: %s", joined)
	}
	// The resources view lists containers by the lerd- prefix, so the name is
	// what puts a running tunnel in front of the user.
	if !strings.Contains(joined, "--name lerd-") {
		t.Errorf("name does not carry the lerd- prefix: %s", joined)
	}
	// A name left behind by a crash must not block the next start.
	if !strings.Contains(joined, "--replace") {
		t.Errorf("a stale name would block a restart: %s", joined)
	}
}

func TestNgrokContainerNameIsPerSiteAndBranch(t *testing.T) {
	if got := ngrokContainerName("acme", ""); got != "lerd-ngrok-acme" {
		t.Errorf("ngrokContainerName(acme) = %q", got)
	}
	// Two tunnels can run at once, so a worktree cannot share the parent's name.
	if got := ngrokContainerName("acme", "feature/x"); got != "lerd-ngrok-acme-feature-x" {
		t.Errorf("ngrokContainerName(acme, feature/x) = %q", got)
	}
	// Whatever a branch or site is called, the result has to be a legal
	// container name.
	got := ngrokContainerName("My App", "feat/../weird name!")
	for _, r := range got {
		if !(r == '-' || r == '.' || r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			t.Fatalf("ngrokContainerName produced an illegal container name %q", got)
		}
	}
}

// A reserved domain is what makes an ngrok URL stable between runs, so it has
// to reach the tool on whichever route runs it.
func TestNgrokReservedDomainReachesBothRoutes(t *testing.T) {
	host := strings.Join(ngrokRunner{domain: "myapp.ngrok.app"}.cmd(5180, "", false, "").Args, " ")
	if !strings.Contains(host, "--url=https://myapp.ngrok.app") {
		t.Errorf("host run is not pinned to the reserved domain: %s", host)
	}
	container := strings.Join(ngrokRunner{container: true, domain: "https://myapp.ngrok.app"}.cmd(5180, "tok", false, "lerd-ngrok-t").Args, " ")
	if !strings.Contains(container, "--url=https://myapp.ngrok.app") {
		t.Errorf("container run is not pinned to the reserved domain: %s", container)
	}
}

// Extra flags are the whole point of the passthrough: a host-header rewrite or
// a traffic policy has no lerd equivalent.
func TestNgrokExtraArgsReachBothRoutes(t *testing.T) {
	extra := []string{"--host-header=rewrite"}
	host := strings.Join(ngrokRunner{extra: extra}.cmd(5180, "", false, "").Args, " ")
	if !strings.Contains(host, "--host-header=rewrite") {
		t.Errorf("host run dropped the extra flags: %s", host)
	}
	container := strings.Join(ngrokRunner{container: true, extra: extra}.cmd(5180, "tok", false, "lerd-ngrok-t").Args, " ")
	if !strings.Contains(container, "--host-header=rewrite") {
		t.Errorf("container run dropped the extra flags: %s", container)
	}
}

func TestSplitNgrokArgs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"--host-header=rewrite --compression", []string{"--host-header=rewrite", "--compression"}},
		// A policy file can sit in a path with spaces, so quoting has to survive
		// the round trip through the config file.
		{`--traffic-policy-file="/home/a b/policy.yml"`, []string{"--traffic-policy-file=/home/a b/policy.yml"}},
		{`--basic-auth 'user:pass word'`, []string{"--basic-auth", "user:pass word"}},
	}
	for _, c := range cases {
		got, err := splitNgrokArgs(c.in)
		if err != nil {
			t.Errorf("splitNgrokArgs(%q) error: %v", c.in, err)
			continue
		}
		if strings.Join(got, "\x00") != strings.Join(c.want, "\x00") {
			t.Errorf("splitNgrokArgs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := splitNgrokArgs(`--basic-auth "unclosed`); err == nil {
		t.Error("an unclosed quote should be refused rather than guessed at")
	}
}

// lerd scrapes ngrok's own log for the public URL, so a run that reshapes that
// log has to be refused at the flag rather than time out later.
func TestNgrokArgsRefuseTheFlagsLerdOwns(t *testing.T) {
	for _, args := range []string{"--log stdout", "--log-format=json"} {
		tool := &shareTool{mode: shareModeNgrok}
		err := applyNgrokArgs(tool, args, "")
		if err == nil || !strings.Contains(err.Error(), "lerd sets") {
			t.Errorf("applyNgrokArgs(%q) error = %v, want it refused", args, err)
		}
	}
}

// Two ways to say the same thing is the one case where a silent winner is worse
// than an error: the loser would be a public URL the user is still expecting.
func TestNgrokArgsRefuseADomainAlreadySetByTheFlag(t *testing.T) {
	tool := &shareTool{mode: shareModeNgrok, ngrok: ngrokRunner{domain: "myapp.ngrok.app"}}
	err := applyNgrokArgs(tool, "--url=other.ngrok.app", "")
	if err == nil || !strings.Contains(err.Error(), "--domain") {
		t.Errorf("error = %v, want the conflict with --domain reported", err)
	}
}

// A flag given for this run outranks the stored one without replacing it, the
// same way an ngrok token does.
func TestNgrokArgsRunFlagBeatsTheStoredOne(t *testing.T) {
	tool := &shareTool{mode: shareModeNgrok}
	if err := applyNgrokArgs(tool, "--compression", "--host-header=rewrite"); err != nil {
		t.Fatalf("applyNgrokArgs() error: %v", err)
	}
	if got := strings.Join(tool.ngrok.extra, " "); got != "--compression" {
		t.Errorf("extra = %q, want the run flag alone", got)
	}
}

// Stored args are configuration, not a request: another tool being picked is
// not a mistake worth failing a share over. Asking for them for this run is.
func TestNgrokArgsOnlyApplyToNgrok(t *testing.T) {
	tool := &shareTool{mode: shareModeCloudflare}
	if err := applyNgrokArgs(tool, "", "--host-header=rewrite"); err != nil {
		t.Errorf("stored args should be ignored for another tool, got: %v", err)
	}
	if err := applyNgrokArgs(tool, "--host-header=rewrite", ""); err == nil {
		t.Error("--ngrok-args on a non-ngrok share should have been refused")
	}
}

// A path only means something on the host it came from, so the container route
// has to carry the file in or the flag is a silent no-op.
func TestNgrokContainerMountsFileValuedFlags(t *testing.T) {
	dir := t.TempDir()
	policy := dir + "/policy.yml"
	if err := os.WriteFile(policy, []byte("inbound: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := ngrokRunner{container: true, extra: []string{"--traffic-policy-file", policy}}
	joined := strings.Join(runner.cmd(5180, "tok", false, "lerd-ngrok-t").Args, " ")

	if !strings.Contains(joined, "-v "+policy+":"+ngrokContainerFileDir+"/policy.yml:ro") {
		t.Errorf("the policy file was not mounted into the container: %s", joined)
	}
	if !strings.Contains(joined, "--traffic-policy-file "+ngrokContainerFileDir+"/policy.yml") {
		t.Errorf("the flag still points at a host path the container cannot see: %s", joined)
	}
}

// A file that is not there would surface as an opaque ngrok start failure, and
// on the container route as a directory podman creates out of thin air.
func TestNgrokArgsRejectAMissingFile(t *testing.T) {
	tool := &shareTool{mode: shareModeNgrok}
	err := applyNgrokArgs(tool, "--traffic-policy-file=/no/such/policy.yml", "")
	if err == nil || !strings.Contains(err.Error(), "policy.yml") {
		t.Errorf("error = %v, want the missing file named", err)
	}
}
