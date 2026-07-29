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
	for _, want := range []string{"--net=host", "-e", "NGROK_AUTHTOKEN", ngrokImage, "http", "5180"} {
		if !strings.Contains(joined, want) {
			t.Errorf("container command missing %q: %s", want, joined)
		}
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
