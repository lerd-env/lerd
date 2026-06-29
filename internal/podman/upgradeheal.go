package podman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// PodmanEnv fingerprints the host podman that lerd last installed against.
// Stored at install/start so the next run can detect a podman upgrade (which on
// rootless Linux silently reshuffles storage and networking) and self-heal
// before the user hits the cryptic "failed to mount runtime directory for
// rootless netns" wall that motivated this (#635).
type PodmanEnv struct {
	Version        string `json:"version"` // full token, e.g. "4.6.2"
	Major          int    `json:"major"`
	Minor          int    `json:"minor"`
	NetworkBackend string `json:"networkBackend"` // "netavark" | "cni" | ""
}

func podmanEnvPath() string {
	return filepath.Join(config.DataDir(), "podman-env.json")
}

// LoadPodmanEnv returns the last recorded fingerprint, or the zero value when
// none has been written yet (a fresh install, or a pre-fingerprint lerd).
func LoadPodmanEnv() PodmanEnv {
	data, err := os.ReadFile(podmanEnvPath())
	if err != nil {
		return PodmanEnv{}
	}
	var env PodmanEnv
	if err := json.Unmarshal(data, &env); err != nil {
		return PodmanEnv{}
	}
	return env
}

// SavePodmanEnv records the current fingerprint for the next run to compare.
func SavePodmanEnv(env PodmanEnv) error {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.DataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(podmanEnvPath(), data, 0o644)
}

// CurrentPodmanEnv probes the live podman for its version and network backend
// in a single `podman info` call (the server-side values, which on rootless
// Linux are the runtime that actually matters).
func CurrentPodmanEnv() (PodmanEnv, error) {
	out, err := execCommand(PodmanBin(), "info", "--format",
		"{{.Version.Version}}|{{.Host.NetworkBackend}}").Output()
	if err != nil {
		return PodmanEnv{}, err
	}
	ver, backend, _ := strings.Cut(strings.TrimSpace(string(out)), "|")
	major, minor, err := splitMajorMinor(ver)
	if err != nil {
		return PodmanEnv{}, err
	}
	return PodmanEnv{
		Version:        cleanVersionToken(strings.TrimSpace(ver)),
		Major:          major,
		Minor:          minor,
		NetworkBackend: strings.TrimSpace(backend),
	}, nil
}

// NetworkHelpers reports the filesystem paths podman resolves for its rootless
// network helpers (netavark, aardvark-dns). These live in libexec, not on
// $PATH, so podman's own resolution is the only reliable probe. An empty path
// means podman cannot find that helper; probed is false when podman could not
// be queried (e.g. an older podman without the template field), so callers skip
// the check rather than report a false failure.
func NetworkHelpers() (netavark, aardvark string, probed bool) {
	out, err := execCommand(PodmanBin(), "info", "--format",
		"{{.Host.NetworkBackendInfo.Path}}|{{.Host.NetworkBackendInfo.DNS.Path}}").Output()
	if err != nil {
		return "", "", false
	}
	nv, ad, ok := strings.Cut(strings.TrimSpace(string(out)), "|")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(nv), strings.TrimSpace(ad), true
}

// upgradeNeedsHeal decides whether a podman change since the last install
// warrants the migrate/netns/network heal. A major-version change or a
// network-backend switch both reshuffle rootless storage and networking; a
// minor/patch bump does not. Pure, so the policy is unit-testable.
func upgradeNeedsHeal(prev, cur PodmanEnv) (bool, string) {
	if prev.Version == "" {
		return false, "" // first install — nothing to compare against
	}
	if prev.Major != cur.Major {
		return true, fmt.Sprintf("podman %s → %s", prev.Version, cur.Version)
	}
	if prev.NetworkBackend != "" && cur.NetworkBackend != "" &&
		prev.NetworkBackend != cur.NetworkBackend {
		return true, fmt.Sprintf("network backend %s → %s", prev.NetworkBackend, cur.NetworkBackend)
	}
	return false, ""
}

// rootlessNetnsDir is the stale shared rootless network namespace a podman major
// upgrade leaves behind; mounting it is what fails after the upgrade. Empty on
// hosts without XDG_RUNTIME_DIR, making the clear a natural no-op.
func rootlessNetnsDir() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "containers", "networks", "rootless-netns")
}

func clearRootlessNetns() error {
	p := rootlessNetnsDir()
	if p == "" {
		return nil
	}
	if _, err := os.Stat(p); err != nil {
		return nil // not present — nothing to clear
	}
	return os.RemoveAll(p)
}

func systemMigrate() error {
	return execCommand(PodmanBin(), "system", "migrate").Run()
}

// HealPodmanUpgrade detects a podman upgrade since the last install and, when
// found, runs the remediation that unblocked #635: rebuild the lerd bridge in
// the new backend's format, migrate rootless storage, and clear the stale
// rootless-netns. emit, when non-nil, narrates each step.
//
// restart lists the containers torn down by the network rebuild so the caller
// can bring them back up; it is returned even on a partial failure so nothing
// is left stopped. The current fingerprint is recorded up front, so a failed
// heal warns and points at manual steps rather than re-running its destructive
// steps on every subsequent invocation. Linux-only: on macOS the runtime lives
// in a VM and the rootless-netns failure mode does not exist.
func HealPodmanUpgrade(dns []string, emit func(string)) (healed bool, restart []string, err error) {
	if runtime.GOOS != "linux" {
		return false, nil, nil
	}
	cur, perr := CurrentPodmanEnv()
	if perr != nil {
		return false, nil, nil // can't probe podman — don't block install over it
	}
	prev := LoadPodmanEnv()
	heal, reason := upgradeNeedsHeal(prev, cur)

	// Record the new fingerprint before doing anything destructive. The heal is
	// best-effort; persisting up front makes it at-most-once per podman version
	// so a mid-heal failure can't re-tear-down containers on every later run.
	_ = SavePodmanEnv(cur)
	if !heal {
		return false, nil, nil
	}

	if emit != nil {
		emit(reason)
		emit("recreating lerd network")
	}
	// Rebuild the bridge first: RecreateNetwork stops and force-removes the lerd
	// containers, so `podman system migrate` below runs with them down (its
	// documented precondition), and attached tells the caller what to restart.
	attached, _, rErr := RecreateNetwork("lerd", dns)
	if rErr != nil {
		return false, attached, fmt.Errorf("recreate lerd network: %w", rErr)
	}
	if emit != nil {
		emit("podman system migrate")
	}
	if mErr := systemMigrate(); mErr != nil {
		return false, attached, fmt.Errorf("podman system migrate: %w", mErr)
	}
	if emit != nil {
		emit("clearing stale rootless-netns")
	}
	if cErr := clearRootlessNetns(); cErr != nil {
		return false, attached, fmt.Errorf("clear rootless-netns: %w", cErr)
	}
	return true, attached, nil
}
