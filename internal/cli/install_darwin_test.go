//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPodmanHelperInstalled(t *testing.T) {
	tests := []struct {
		name     string
		plist    string
		username string
		want     bool
	}{
		{"per-user plist", "com.github.containers.podman.helper-george.plist", "george", true},
		{"unsuffixed plist", "com.github.containers.podman.helper.plist", "george", true},
		{"other user's plist", "com.github.containers.podman.helper-alice.plist", "george", false},
		{"not installed", "", "george", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.plist != "" {
				if err := os.WriteFile(filepath.Join(dir, tc.plist), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if got := podmanHelperInstalled(dir, tc.username); got != tc.want {
				t.Errorf("podmanHelperInstalled = %v, want %v", got, tc.want)
			}
		})
	}
}
