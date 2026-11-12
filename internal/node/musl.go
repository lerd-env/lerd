package node

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/download"
)

// lerd's PHP containers are Alpine, where the host's glibc Node cannot run, so
// a shell in one would otherwise get the image's own Node, whatever version
// Alpine ships. Node publishes musl builds of each release here instead.
var muslRelease = "https://unofficial-builds.nodejs.org/download/release"

// exactVersion is a release as process.version prints it; it ends up in a URL
// and a folder name.
var exactVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// MuslRoot holds one musl build of Node per version. The containers mount the
// home folder at its own path, so a build here runs in them as it is.
func MuslRoot() string { return filepath.Join(config.DataDir(), "node-musl") }

// muslArch is Node's name for the container's architecture, which is the
// host's: on macOS the Podman machine runs the Mac's own.
func muslArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	}
	return "", fmt.Errorf("no musl build of Node for %s", goarch)
}

// MuslBin is the bin folder of the musl build of version ("v22.23.3"),
// downloading it on first use and checking it against the release's checksums.
func MuslBin(ctx context.Context, version string, w io.Writer) (string, error) {
	if !exactVersion.MatchString(version) {
		return "", fmt.Errorf("%q is not a Node version", version)
	}
	dir := filepath.Join(MuslRoot(), version)
	bin := filepath.Join(dir, "bin")
	if _, err := os.Stat(filepath.Join(bin, "node")); err == nil {
		return bin, nil
	}
	arch, err := muslArch(runtime.GOARCH)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("node-%s-linux-%s-musl.tar.gz", version, arch)
	base := muslRelease + "/" + version
	if err := os.MkdirAll(MuslRoot(), 0o755); err != nil {
		return "", err
	}
	// Each call works in its own folder, so two first shells for one version
	// cannot delete each other's downloads.
	tmp, err := os.MkdirTemp(MuslRoot(), version+"-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	sums := filepath.Join(tmp, "SHASUMS256.txt")
	if err := download.File(ctx, base+"/SHASUMS256.txt", sums, 0o644, io.Discard); err != nil {
		return "", fmt.Errorf("Node %s has no musl build: %w", version, err)
	}
	digest, err := sumFor(sums, name)
	if err != nil {
		return "", err
	}
	archive := filepath.Join(tmp, name)
	if err := download.Verified(ctx, base+"/"+name, archive, 0o644, digest, w); err != nil {
		return "", err
	}
	tree := filepath.Join(tmp, "tree")
	if err := extractTree(archive, tree, true); err != nil {
		return "", err
	}
	if err := os.Rename(tree, dir); err != nil {
		// Another call finished the same version first.
		if _, statErr := os.Stat(filepath.Join(bin, "node")); statErr != nil {
			return "", err
		}
	}
	return bin, nil
}

// sumFor reads name's sha256 out of a SHASUMS256.txt.
func sumFor(path, name string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if fields := strings.Fields(s.Text()); len(fields) == 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("%s is not in the release's checksums, so this platform has no musl build of it", name)
}

// extractTree unpacks a tarball into dest, dropping the top folder Node and npm
// tarballs wrap their tree in. Nothing may land outside dest; links refuses
// symlinks when false, for archives that never need one.
func extractTree(archive, dest string, links bool) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(hdr.Name, "/")
		if !ok || rel == "" {
			continue
		}
		target := filepath.Join(dest, rel)
		if !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("%s leaves the archive's folder", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			err = writeFile(target, tr, os.FileMode(hdr.Mode).Perm())
		case tar.TypeSymlink:
			if !links {
				return fmt.Errorf("%s is a symlink, which this archive should not carry", hdr.Name)
			}
			if filepath.IsAbs(hdr.Linkname) || !strings.HasPrefix(filepath.Join(filepath.Dir(target), hdr.Linkname), dest+string(os.PathSeparator)) {
				return fmt.Errorf("%s links outside the archive's folder", hdr.Name)
			}
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				err = os.Symlink(hdr.Linkname, target)
			}
		}
		if err != nil {
			return err
		}
	}
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil { //nolint:gosec // checksum-verified release archive
		f.Close()
		return err
	}
	return f.Close()
}

// specFile records the version the host resolved a site's spec to: "lts/iron"
// names none, and two sites on "default" can differ through their mise.toml.
func specFile(site, spec string) string {
	return filepath.Join(MuslRoot(), "spec", url.PathEscape(site), url.PathEscape(spec))
}

// RememberSpec records that spec resolved to version on the host for site.
func RememberSpec(site, spec, version string) error {
	f := specFile(site, spec)
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f, []byte(version+"\n"), 0o644)
}

// MuslBinFor is the build lerd shell last fetched for site's spec, else the
// newest matching a numeric spec, else "". It never downloads, so shims stay quick.
func MuslBinFor(site, spec string) string {
	if spec == "" {
		return ""
	}
	if b, err := os.ReadFile(specFile(site, spec)); err == nil {
		bin := filepath.Join(MuslRoot(), strings.TrimSpace(string(b)), "bin")
		if _, err := os.Stat(filepath.Join(bin, "node")); err == nil {
			return bin
		}
	}
	spec = strings.TrimPrefix(spec, "v")
	entries, _ := os.ReadDir(MuslRoot())
	best := ""
	for _, e := range entries {
		v := e.Name()
		if !e.IsDir() || !exactVersion.MatchString(v) || (v != "v"+spec && !strings.HasPrefix(v, "v"+spec+".")) {
			continue
		}
		if best == "" || newerVersion(v, best) {
			best = v
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(MuslRoot(), best, "bin")
}

// newerVersion compares two "vX.Y.Z" names numerically.
func newerVersion(a, b string) bool {
	pa, pb := strings.Split(a[1:], "."), strings.Split(b[1:], ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x > y
		}
	}
	return len(pa) > len(pb)
}
