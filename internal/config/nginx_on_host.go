package config

import (
	"os"
	"path/filepath"
)

func nginxOnHostMarker() string { return filepath.Join(DataDir(), "nginx-on-host") }

// NginxOnHost reports whether nginx was last placed on the host rather than in
// its container. The nginx package decides the placement; this only records it.
func NginxOnHost() bool {
	_, err := os.Stat(nginxOnHostMarker())
	return err == nil
}

// SetNginxOnHost records where nginx runs.
func SetNginxOnHost(on bool) error {
	if !on {
		if err := os.Remove(nginxOnHostMarker()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(nginxOnHostMarker(), nil, 0o644)
}
