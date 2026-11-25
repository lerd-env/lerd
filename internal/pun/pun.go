// Package pun installs the pun PHP toolchain (https://punphp.com) at the
// version tools.yaml pins. It is a static binary, so it runs on the host; the
// php it calls for composer scripts is lerd's shim, which reaches the site's PHP.
package pun

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/geodro/lerd/internal/download"
	"github.com/geodro/lerd/internal/tools"
)

const name = "pun"

// fetch is the verified download, a seam so the install is testable offline.
var fetch = download.Verified

// Path is where lerd keeps its pun binary.
func Path() string { return tools.Path(name) }

// Ensure installs the pinned pun when it is missing or not the pinned version,
// and answers where it is. Progress goes to w.
func Ensure(ctx context.Context, w io.Writer) (string, error) {
	m := tools.Load(ctx)
	pin, ok := m.Tools[name]
	if !ok {
		return "", fmt.Errorf("lerd pins no pun version; run lerd update")
	}
	dest := Path()
	if _, err := os.Stat(dest); err == nil && tools.InstalledVersion(name) == pin.Version {
		return dest, nil
	}
	url, err := m.URL(name, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(w, "Downloading pun %s\n", pin.Version)
	// Per call, so two first runs at once never share or delete each other's.
	tmp, err := os.CreateTemp(filepath.Dir(dest), "pun-*.tar.gz")
	if err != nil {
		return "", err
	}
	archive := tmp.Name()
	tmp.Close()
	defer os.Remove(archive)
	if err := fetch(ctx, url, archive, 0o644, m.Digest(name, runtime.GOOS, runtime.GOARCH), w); err != nil {
		return "", fmt.Errorf("pun download: %w", err)
	}
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := download.ExtractTarGzBinary(f, name, dest); err != nil {
		return "", fmt.Errorf("pun extract: %w", err)
	}
	// Stamped last, so a half-installed binary is fetched again next time.
	return dest, tools.WriteStamp(name, pin.Version)
}
