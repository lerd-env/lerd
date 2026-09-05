package cli

import "testing"

// A native site's workers must find the native php before BinDir's container
// shim, so the shim dir is prepended rather than appended.
func TestWorkerBinDirs(t *testing.T) {
	cases := []struct {
		name   string
		phpDir string
		extra  string
		want   string
	}{
		{"native only", "/run/native/8.4/bin", "", "/run/native/8.4/bin"},
		{"native wins over node", "/run/native/8.4/bin", "/node/bin", "/run/native/8.4/bin:/node/bin"},
		{"container site keeps node dirs", "", "/node/bin", "/node/bin"},
		{"neither", "", "", ""},
	}
	for _, c := range cases {
		if got := workerBinDirs(c.phpDir, c.extra); got != c.want {
			t.Errorf("%s: workerBinDirs(%q,%q) = %q, want %q", c.name, c.phpDir, c.extra, got, c.want)
		}
	}
}
