package cli

import "github.com/geodro/lerd/internal/podman"

// The VM-side scripts for macOS, kept free of build tags so they are tested on
// every platform. Site names reach them only after validProvidedEnvSite.

func providedEnvSSHArgs(machine, script string) []string {
	return []string{"machine", "ssh", machine, script}
}

func providedEnvMkdirScript() string {
	return "sudo sh -c 'umask 077; mkdir -p " + podman.ProvidedEnvVMDir + "'"
}

// providedEnvWriteScript writes stdin to a temp file and renames it into place,
// so PHP never reads a half-written file.
func providedEnvWriteScript(siteName string) string {
	dir := podman.ProvidedEnvVMDir
	tmp := dir + "/.provided-" + siteName
	return "sudo sh -c 'umask 077; mkdir -p " + dir + " && cat > " + tmp + " && mv " + tmp + " " + dir + "/" + siteName + ".env'"
}

func providedEnvRemoveScript(siteName string) string {
	return "sudo rm -f " + podman.ProvidedEnvVMDir + "/" + siteName + ".env"
}

// providedEnvListScript prints the dir's files, nothing when it is missing.
func providedEnvListScript() string {
	return "sudo ls " + podman.ProvidedEnvVMDir + " 2>/dev/null || true"
}
