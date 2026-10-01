package config

import "testing"

// A database cannot keep its files on a Windows share: SQLite and friends fail
// with disk I/O errors over 9p. There the data lives in a named volume on the
// VM's own disk; everywhere else it stays a host directory.
func TestDataVolumeSource(t *testing.T) {
	const host = "/home/me/.local/share/lerd/data/mysql"
	cases := []struct{ goos, want string }{
		{"linux", host},
		{"darwin", host},
		{"windows", "lerd-data-mysql"},
	}
	for _, c := range cases {
		if got := dataVolumeSource(c.goos, "mysql", host); got != c.want {
			t.Errorf("dataVolumeSource(%q) = %q, want %q", c.goos, got, c.want)
		}
	}
}

func TestDataVolumeIsNamedOnlyWhereItReplacesAHostDir(t *testing.T) {
	if !usesNamedDataVolume("windows") || usesNamedDataVolume("linux") || usesNamedDataVolume("darwin") {
		t.Error("only Windows keeps service data in a named volume")
	}
	if DataVolumeName("my-svc") != "lerd-data-my-svc" {
		t.Errorf("volume name = %q", DataVolumeName("my-svc"))
	}
}
