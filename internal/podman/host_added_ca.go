package podman

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
)

// hostCAPrefix names the roots staged into the build context, so one COPY glob
// picks them all up and nothing else.
const hostCAPrefix = "host-ca-"

// hostCASource is a folder where the host keeps roots it trusts on top of the
// distro's set. Debian's update-ca-certificates only reads *.crt there.
type hostCASource struct {
	dir     string
	crtOnly bool
}

// hostAddedCASources are the local anchor folders of each distro family. A var
// so tests can point it at fixtures.
var hostAddedCASources = []hostCASource{
	{dir: "/usr/local/share/ca-certificates", crtOnly: true}, // Debian, Ubuntu
	{dir: "/etc/pki/ca-trust/source/anchors"},                // Fedora, RHEL
	{dir: "/etc/ca-certificates/trust-source/anchors"},       // Arch
	{dir: "/etc/pki/trust/anchors"},                          // openSUSE, admin-added
	{dir: "/usr/share/pki/trust/anchors"},                    // openSUSE, where lerd puts its CA
}

// hostAddedCABlock stages the host's added roots into tmpDir and returns the step
// trusting them, or "" when there are none or php:trust-host-ca is off. Only
// parsed certificates are written out, so a stray file adds nothing else.
func hostAddedCABlock(tmpDir string) string {
	if cfg, err := config.LoadGlobal(); err != nil || !cfg.PHP.TrustHostCA {
		return ""
	}
	staged := 0
	for _, src := range hostAddedCASources {
		_ = filepath.WalkDir(src.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !wantHostCAFile(d.Name(), src.crtOnly) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			for _, der := range certificatesIn(data) {
				if stageHostCA(tmpDir, der) {
					staged++
				}
			}
			return nil
		})
	}
	if staged == 0 {
		return ""
	}
	return "# CA roots the host added locally are trusted here too\n" +
		"COPY " + hostCAPrefix + "*.crt /usr/local/share/ca-certificates/\n" +
		"RUN update-ca-certificates\n"
}

// wantHostCAFile skips lerd's own CA, which mkcertCABlock already adds, and on
// Debian anything update-ca-certificates would ignore.
func wantHostCAFile(name string, crtOnly bool) bool {
	if strings.HasPrefix(name, "lerd-mkcert-rootCA.") {
		return false
	}
	return !crtOnly || strings.HasSuffix(name, ".crt")
}

// certificatesIn returns every certificate in a PEM file, or the file itself
// when it is a single DER certificate, as anchor folders accept both.
func certificatesIn(data []byte) [][]byte {
	var ders [][]byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err == nil {
			ders = append(ders, block.Bytes)
		}
	}
	if len(ders) == 0 {
		if _, err := x509.ParseCertificate(data); err == nil {
			ders = append(ders, data)
		}
	}
	return ders
}

// stageHostCA writes one root as PEM, named by its hash so the same root found
// in two folders is staged once. Reports whether a new file was written.
func stageHostCA(tmpDir string, der []byte) bool {
	sum := sha256.Sum256(der)
	dest := filepath.Join(tmpDir, hostCAPrefix+hex.EncodeToString(sum[:8])+".crt")
	if _, err := os.Stat(dest); err == nil {
		return false
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return os.WriteFile(dest, out, 0o644) == nil
}

// trustHostCAsFirst puts the host-roots step straight after every FROM, so a
// proxy that intercepts build downloads is trusted before the first apk or pecl
// call. Done here rather than in the templates so their hashes stay put.
func trustHostCAsFirst(containerfile, block string) string {
	if block == "" {
		return containerfile
	}
	lines := strings.SplitAfter(containerfile, "\n")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		if strings.HasPrefix(line, "FROM ") {
			if !strings.HasSuffix(line, "\n") {
				b.WriteString("\n")
			}
			b.WriteString(block)
		}
	}
	return b.String()
}
