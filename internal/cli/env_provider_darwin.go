//go:build darwin

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// On macOS the FPM containers run in the Podman Machine VM, so the provided env
// lives in the VM's tmpfs. lerd writes it over `podman machine ssh` on stdin:
// the values go from the provider's stdout into VM memory and never touch the
// Mac's disk or an argv. lerd's containers run rootful in the VM, hence sudo.

func providedEnvSupported() error {
	if cfg, err := config.LoadGlobal(); err == nil && cfg.PHPRuntimeMode() == config.PHPRuntimeNative {
		return errors.New("env_provider is not supported on the native PHP runtime yet")
	}
	if selectedMachineName() == "" {
		return errors.New("env_provider needs the Podman Machine to be set up")
	}
	return nil
}

// ensureProvidedEnvDir creates the VM dir the FPM containers mount. The launchd
// units have no ExecStartPre, and podman refuses a missing bind source, so this
// runs on lerd start before any container comes up.
func ensureProvidedEnvDir() {
	if providedEnvSupported() != nil {
		return
	}
	if err := providedEnvSSH(providedEnvMkdirScript(), nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "creating %s in the Podman Machine: %v\n", podman.ProvidedEnvVMDir, err)
	}
}

func storeProvidedEnv(siteName string, data []byte) error {
	if err := providedEnvSSH(providedEnvWriteScript(siteName), data, nil); err != nil {
		return err
	}
	providedEnvListing.add(siteName + ".env")
	return nil
}

func dropProvidedEnv(siteName string) {
	if !validProvidedEnvSite(siteName) || providedEnvSupported() != nil {
		return
	}
	dropProvidedEnvVM(siteName)
}

// dropProvidedEnvVM skips the ssh only when the current pass listed the dir and
// the file is not there. Outside a pass it always removes: lerd-ui and the
// watcher live for days, and another process may have written it since.
func dropProvidedEnvVM(siteName string) {
	file := siteName + ".env"
	if listed, has := providedEnvListing.has(file); listed && !has {
		return
	}
	if providedEnvSSH(providedEnvRemoveScript(siteName), nil, nil) == nil {
		providedEnvListing.remove(file)
	}
}

// beginProvidedEnvPass lists the VM dir once for a pass over every site, so
// sites without a provider cost no ssh each. Call the returned func to end it.
func beginProvidedEnvPass() func() {
	if providedEnvSupported() != nil {
		return func() {}
	}
	var out bytes.Buffer
	if err := providedEnvSSH(providedEnvListScript(), nil, &out); err != nil {
		return func() {}
	}
	files := map[string]bool{}
	for _, f := range strings.Fields(out.String()) {
		files[f] = true
	}
	providedEnvListing.set(files)
	return func() { providedEnvListing.set(nil) }
}

// providedEnvListing is locked because lerd-ui can unlink while a start runs.
var providedEnvListing vmListing

type vmListing struct {
	mu    sync.Mutex
	files map[string]bool
}

func (l *vmListing) set(files map[string]bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.files = files
}

func (l *vmListing) has(file string) (listed, has bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.files != nil, l.files[file]
}

func (l *vmListing) add(file string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.files != nil {
		l.files[file] = true
	}
}

func (l *vmListing) remove(file string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.files, file)
}

// providedEnvSSH is a var so tests can stand in for the Podman Machine.
var providedEnvSSH = func(script string, stdin []byte, stdout *bytes.Buffer) error {
	cmd := podman.Cmd(providedEnvSSHArgs(selectedMachineName(), script)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}
