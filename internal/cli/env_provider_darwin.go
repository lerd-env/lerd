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
	if files := providedEnvVMFiles(); files != nil {
		files[siteName+".env"] = true
	}
	return nil
}

// dropProvidedEnv only reaches into the VM for a file that is there, so sites
// without a provider cost no ssh round-trip each on lerd start and lerd env.
func dropProvidedEnv(siteName string) {
	if !validProvidedEnvSite(siteName) || providedEnvSupported() != nil {
		return
	}
	files := providedEnvVMFiles()
	if files != nil && !files[siteName+".env"] {
		return
	}
	if providedEnvSSH(providedEnvRemoveScript(siteName), nil, nil) == nil && files != nil {
		delete(files, siteName+".env")
	}
}

// providedEnvVMFiles lists the VM's provided-env dir once per process. nil when
// the listing failed, so callers fall back to trying the removal anyway.
var providedEnvVMFiles = sync.OnceValue(func() map[string]bool {
	var out bytes.Buffer
	if err := providedEnvSSH(providedEnvListScript(), nil, &out); err != nil {
		return nil
	}
	files := map[string]bool{}
	for _, f := range strings.Fields(out.String()) {
		files[f] = true
	}
	return files
})

func providedEnvSSH(script string, stdin []byte, stdout *bytes.Buffer) error {
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
