//go:build darwin

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMachineInitArgs(t *testing.T) {
	// Every required mount must be passed explicitly, or Podman's stringArray
	// --volume drops the defaults (the lerd <= 1.24.0 bug).
	args := machineInitArgs("", 4096, "")
	joined := strings.Join(args, " ")
	for _, m := range requiredMachineMounts {
		if !strings.Contains(joined, "-v "+m+":"+m) {
			t.Errorf("init args missing mount %q: %v", m, args)
		}
	}
	if !strings.Contains(joined, "--rootful") {
		t.Errorf("init args missing --rootful: %v", args)
	}
	if !strings.Contains(joined, "--memory 4096") {
		t.Errorf("init args missing memory: %v", args)
	}
	// An empty provider must not emit --provider (older podman where applehv is
	// the default and the flag may not exist).
	if strings.Contains(joined, "--provider") {
		t.Errorf("empty provider should omit --provider: %v", args)
	}
	// A name (recreate path) must be the trailing positional arg.
	named := machineInitArgs("podman-machine-default", 0, "")
	if named[len(named)-1] != "podman-machine-default" {
		t.Errorf("named init args should end with the machine name: %v", named)
	}
	if strings.Contains(strings.Join(named, " "), "--memory") {
		t.Errorf("zero memory should omit --memory: %v", named)
	}
	// Pinning a provider (podman 6: avoid the libkrun/krunkit default) emits
	// --provider, with the name still trailing.
	pinned := machineInitArgs("podman-machine-default", 0, "applehv")
	if !strings.Contains(strings.Join(pinned, " "), "--provider applehv") {
		t.Errorf("pinned provider should emit --provider applehv: %v", pinned)
	}
	if pinned[len(pinned)-1] != "podman-machine-default" {
		t.Errorf("pinned init args should still end with the machine name: %v", pinned)
	}
}

func TestMachineMissingHomeMount(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	machineDir := filepath.Join(tmp, ".config", "containers", "podman", "machine", "applehv")
	if err := os.MkdirAll(machineDir, 0755); err != nil {
		t.Fatal(err)
	}

	write := func(name string, sources []string) {
		mounts := make([]any, len(sources))
		for i, s := range sources {
			mounts[i] = map[string]any{"Source": s, "Target": s, "Type": "virtiofs"}
		}
		data, _ := json.Marshal(map[string]any{"Name": name, "Mounts": mounts})
		if err := os.WriteFile(filepath.Join(machineDir, name+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name    string
		sources []string
		want    bool
	}{
		// The lerd <= 1.24.0 broken machine: only /Volumes, no home mount.
		{"broken-volumes-only", []string{"/Volumes"}, true},
		{"healthy-defaults", []string{"/Users", "/private", "/var/folders"}, false},
		{"healthy-all", []string{"/Users", "/private", "/var/folders", "/Volumes"}, false},
		{"empty", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			write(c.name, c.sources)
			if got := machineMissingHomeMount(c.name); got != c.want {
				t.Errorf("machineMissingHomeMount(%v) = %v, want %v", c.sources, got, c.want)
			}
		})
	}

	// A machine config that can't be read must not be reported broken (we never
	// recreate on uncertainty).
	if machineMissingHomeMount("does-not-exist") {
		t.Error("missing config should not be reported as broken")
	}
}

func TestMachineMissingMounts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	machineDir := filepath.Join(tmp, ".config", "containers", "podman", "machine", "applehv")
	if err := os.MkdirAll(machineDir, 0755); err != nil {
		t.Fatal(err)
	}

	write := func(name string, sources []string) {
		mounts := make([]any, len(sources))
		for i, s := range sources {
			mounts[i] = map[string]any{"Source": s, "Target": s, "Type": "virtiofs"}
		}
		data, _ := json.Marshal(map[string]any{"Name": name, "Mounts": mounts})
		if err := os.WriteFile(filepath.Join(machineDir, name+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name    string
		sources []string
		want    []string
	}{
		// A machine created before lerd asked for /Volumes keeps Podman's own
		// defaults, so an external drive is never shared with the VM (#1725).
		{"podman-defaults", []string{"/Users", "/private", "/var/folders"}, []string{"/Volumes"}},
		{"complete", []string{"/Users", "/private", "/var/folders", "/Volumes"}, nil},
		{"home-only", []string{"/Users"}, []string{"/private", "/var/folders", "/Volumes"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			write(c.name, c.sources)
			got := machineMissingMounts(c.name)
			if !slices.Equal(got, c.want) {
				t.Errorf("machineMissingMounts(%v) = %v, want %v", c.sources, got, c.want)
			}
		})
	}

	// A config we cannot read tells us nothing, so nothing is reported missing.
	if got := machineMissingMounts("does-not-exist"); got != nil {
		t.Errorf("unreadable config reported %v missing, want none", got)
	}
}

// The advisory speaks only to installs that serve something from outside the
// home, since that is the only place a missing mount changes anything.
func TestOutOfHomePaths(t *testing.T) {
	home := t.TempDir()
	inside := filepath.Join(home, "Sites")
	outside := "/Volumes/Dock/projects"
	got := outOfHomePaths(home, []string{inside, outside, home, ""})
	if !slices.Equal(got, []string{outside}) {
		t.Errorf("outOfHomePaths = %v, want %v", got, []string{outside})
	}
}
