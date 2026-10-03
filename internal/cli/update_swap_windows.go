//go:build windows

package cli

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func releaseArchiveName(ver, arch string) string {
	return fmt.Sprintf("lerd_%s_windows_%s.zip", ver, arch)
}

// extractReleaseArchive unpacks the release zip, which is flat, into dir. An
// entry carrying a path is refused rather than written outside dir.
func extractReleaseArchive(archive, dir string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name != filepath.Base(f.Name) || f.Name == ".." {
			return fmt.Errorf("refusing archive entry %q", f.Name)
		}
		if err := extractZipFile(f, filepath.Join(dir, f.Name)); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dest string) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// swapBinary replaces dest with a copy of src. Windows will not overwrite or
// delete a running exe but will rename it, so dest is moved aside first and
// the copies earlier swaps moved aside go once nothing runs them.
func swapBinary(src, dest string) error {
	stale, _ := filepath.Glob(dest + ".old-*")
	for _, old := range stale {
		os.Remove(old) //nolint:errcheck
	}
	aside := ""
	if _, err := os.Stat(dest); err == nil {
		aside = fmt.Sprintf("%s.old-%d", dest, time.Now().UnixNano())
		if err := os.Rename(dest, aside); err != nil {
			return err
		}
	}
	if err := copyFile(src, dest, 0o755); err != nil {
		if aside != "" {
			os.Remove(dest)        //nolint:errcheck
			os.Rename(aside, dest) //nolint:errcheck
		}
		return err
	}
	return nil
}
