package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/tools"
)

// refreshProvidedEnv runs the project's env_provider on the host and keeps its
// dotenv output in the site's tmpfs file, which the PHP prepend loads for web
// requests, workers and CLI alike. A project without a provider has any stale
// file removed so secrets do not outlive the setting. approve records consent
// up front, for `lerd env --yes` where there is no terminal to prompt on.
func refreshProvidedEnv(site config.Site, approve bool) error {
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		return fmt.Errorf("env_provider: reading .lerd.yaml: %w", err)
	}
	if proj == nil || proj.EnvProvider == "" {
		dropProvidedEnv(site.Name)
		return nil
	}
	if err := providedEnvSupported(); err != nil {
		return err
	}
	if !validProvidedEnvSite(site.Name) {
		return fmt.Errorf("env_provider: site name %q cannot name a provided env file", site.Name)
	}
	if approve {
		if err := config.ApproveSiteCommand(site.Name, proj.EnvProvider); err != nil {
			return err
		}
	}
	if err := approveHostCommand(site.Name, proj.EnvProvider, "env_provider"); err != nil {
		return err
	}

	var stdout bytes.Buffer
	cmd := exec.Command("/bin/sh", "-c", proj.EnvProvider)
	cmd.Dir = site.Path
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("env_provider failed: %w", err)
	}
	return storeProvidedEnv(site.Name, append(providedEnvHeader(site), stdout.Bytes()...))
}

// validProvidedEnvSite mirrors the prepend's check on LERD_SITE, so a name lerd
// writes is one PHP will read, and it is safe inside a path or a shell word.
func validProvidedEnvSite(name string) bool {
	if name == "" || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// providedEnvHeader names the directories the file may be loaded from: the site
// and its worktrees, then lerd's tool directory, whose scripts count only when
// run from inside one. Any other script, whatever its LERD_SITE, loads nothing.
func providedEnvHeader(site config.Site) []byte {
	roots := []string{site.Path}
	if wts, _ := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain()); len(wts) > 0 {
		for _, wt := range wts {
			roots = append(roots, wt.Path)
		}
	}
	var b bytes.Buffer
	for _, r := range roots {
		writeHeaderDir(&b, "root", r)
	}
	// After the roots: a prepend that predates this line stops reading there.
	// The directory itself only exists once a tool was fetched, so it is the
	// data dir that gets resolved.
	writeHeaderDir(&b, "tools", filepath.Join(config.CanonicalPath(config.DataDir()), filepath.Base(tools.PharDir())))
	return b.Bytes()
}

// writeHeaderDir writes dir under every spelling the PHP containers mount.
func writeHeaderDir(b *bytes.Buffer, key, dir string) {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	for _, spelling := range append([]string{dir}, homeSpellings(dir)...) {
		if spelling == "" || strings.ContainsAny(spelling, "\r\n") {
			continue
		}
		fmt.Fprintf(b, "#lerd-%s=%s\n", key, spelling)
	}
}

// homeSpellingsFn is config.HomeSpellings, swapped in tests for a fake /home.
var homeSpellingsFn = config.HomeSpellings

// homeSpellings returns path under every other spelling of $HOME the PHP
// containers mount, such as /home/<user> for /var/home/<user> on Bazzite, since
// a command run from one spelling never resolves to another inside them.
func homeSpellings(path string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	real := config.CanonicalPath(home)
	if !strings.HasPrefix(path, real+string(filepath.Separator)) {
		return nil
	}
	var out []string
	for _, h := range homeSpellingsFn(home) {
		if h != real {
			out = append(out, h+strings.TrimPrefix(path, real))
		}
	}
	return out
}
