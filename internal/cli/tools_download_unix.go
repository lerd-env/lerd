//go:build !windows

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// fetchMise downloads the pinned mise tarball and extracts the single binary it
// carries at mise/bin/mise to dest, returning the installed version.
func fetchMise(pins *pinnedTools, dest string, w io.Writer) (string, error) {
	tarball := dest + ".tar.gz"
	v, err := pins.download("mise", tarball, 0644, w)
	if err != nil {
		return "", fmt.Errorf("mise download: %w", err)
	}
	defer os.Remove(tarball)
	extract := exec.Command("tar", "xzf", tarball, "-C", filepath.Dir(dest), "--strip-components=2", "mise/bin/mise")
	extract.Stdout = w
	extract.Stderr = w
	if err := extract.Run(); err != nil {
		return "", fmt.Errorf("mise extract: %w", err)
	}
	os.Chmod(dest, 0755) //nolint:errcheck
	return v, nil
}
