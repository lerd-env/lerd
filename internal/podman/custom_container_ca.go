package podman

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

const (
	customCAFile = "lerd-mkcert.crt"
	customCAPath = "/usr/local/share/ca-certificates/" + customCAFile
)

// customCATrustScript adds the CA to whichever trust store the image has. The
// distro's own tool comes first so a later run of it keeps the CA; an image
// without one gets the CA appended to its bundle, or a bundle of its own.
const customCATrustScript = `set -e
ca=` + customCAPath + `
if command -v update-ca-certificates >/dev/null 2>&1; then
    update-ca-certificates
elif command -v update-ca-trust >/dev/null 2>&1; then
    mkdir -p /etc/pki/ca-trust/source/anchors
    cp "$ca" /etc/pki/ca-trust/source/anchors/
    update-ca-trust extract
else
    bundle=/etc/ssl/certs/ca-certificates.crt
    for b in "$bundle" /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/cert.pem; do
        if [ -s "$b" ]; then bundle=$b; break; fi
    done
    mkdir -p "$(dirname "$bundle")"
    { echo; cat "$ca"; } >> "$bundle"
fi
`

// customImageTrustsCA reports whether the site asked for lerd's CA in its image.
func customImageTrustsCA(cfg *config.ContainerConfig) bool {
	return cfg != nil && cfg.TrustCA
}

// customCACert returns the root CA lerd signs site certificates with.
func customCACert() ([]byte, error) {
	root, err := config.CARootDir()
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(filepath.Join(root, "rootCA.pem"))
	if err != nil {
		return nil, fmt.Errorf("container.trust_ca: reading the lerd CA: %w", err)
	}
	return ca, nil
}

// customCAContainerfile layers the CA on baseID, running as root and handing
// the image back to the user it ran as. Node reads neither trust store.
func customCAContainerfile(baseID, user string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\nUSER 0\n", baseID)
	fmt.Fprintf(&b, "COPY %s %s\n", customCAFile, customCAPath)
	b.WriteString("COPY trust.sh /tmp/lerd-trust.sh\n")
	b.WriteString("RUN sh /tmp/lerd-trust.sh && rm /tmp/lerd-trust.sh\n")
	fmt.Fprintf(&b, "ENV NODE_EXTRA_CA_CERTS=%s\n", customCAPath)
	if user != "" {
		fmt.Fprintf(&b, "USER %s\n", user)
	}
	return b.String()
}

// buildCustomImageTrustingCA builds the site's own image untagged and tags only
// the layer carrying the CA, so a failed layer leaves the previous image in place.
func buildCustomImageTrustingCA(imageName, containerfile, buildCtx string, cfg *config.ContainerConfig, stdout, stderr io.Writer) error {
	ca, err := customCACert()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "lerd-custom-ca-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	iidFile := filepath.Join(tmp, "iid")
	if err := runImageBuild(customBaseImageArgs(iidFile, containerfile, buildCtx, cfg), stdout, stderr); err != nil {
		return err
	}
	iid, err := os.ReadFile(iidFile)
	if err != nil {
		return err
	}
	baseID := strings.TrimSpace(string(iid))
	if baseID == "" {
		return fmt.Errorf("podman reported no image ID for the build")
	}
	user, err := execCommand(PodmanBin(), "image", "inspect", "--format", "{{.Config.User}}", baseID).Output()
	if err != nil {
		return fmt.Errorf("reading the image's user: %w", err)
	}

	layer := filepath.Join(tmp, "layer")
	if err := os.Mkdir(layer, 0755); err != nil {
		return err
	}
	for name, body := range map[string][]byte{
		"Containerfile": []byte(customCAContainerfile(baseID, strings.TrimSpace(string(user)))),
		customCAFile:    ca,
		"trust.sh":      []byte(customCATrustScript),
	} {
		if err := os.WriteFile(filepath.Join(layer, name), body, 0644); err != nil {
			return err
		}
	}
	args := []string{"build", "-t", imageName, "-f", filepath.Join(layer, "Containerfile"), layer}
	if err := runImageBuild(args, stdout, stderr); err != nil {
		return fmt.Errorf("trusting the lerd CA (the image needs a shell): %w", err)
	}
	return nil
}

func customBaseImageArgs(iidFile, containerfile, buildCtx string, cfg *config.ContainerConfig) []string {
	args := []string{"build", "--iidfile", iidFile, "-f", containerfile}
	if cfg != nil && cfg.Target != "" {
		args = append(args, "--target", cfg.Target)
	}
	return append(args, buildCtx)
}

func runImageBuild(args []string, stdout, stderr io.Writer) error {
	cmd := execCommand(PodmanBin(), args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
