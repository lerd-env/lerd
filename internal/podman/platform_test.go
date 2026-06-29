package podman

import "testing"

// TestRewriteArm64Image pins the Apple Silicon image swap that fixes #603: only
// postgis/postgis is rewritten to imresamu/postgis, only on arm64, preserving
// the registry prefix and tag. Everything else (mysql:5.7, already-imresamu,
// non-postgis) is left untouched. Pure, so it runs on the Linux CI runner too.
func TestRewriteArm64Image(t *testing.T) {
	cases := []struct {
		name, image, goarch, want string
	}{
		{"arm64 postgis with registry", "docker.io/postgis/postgis:16-3.5-alpine", "arm64", "docker.io/imresamu/postgis:16-3.5-alpine"},
		{"arm64 postgis bare", "postgis/postgis:17-3.6-alpine", "arm64", "imresamu/postgis:17-3.6-alpine"},
		{"amd64 postgis unchanged", "docker.io/postgis/postgis:16-3.5-alpine", "amd64", "docker.io/postgis/postgis:16-3.5-alpine"},
		{"arm64 mysql 5.7 unchanged", "docker.io/library/mysql:5.7", "arm64", "docker.io/library/mysql:5.7"},
		{"arm64 already imresamu unchanged", "docker.io/imresamu/postgis:16-3.5-alpine", "arm64", "docker.io/imresamu/postgis:16-3.5-alpine"},
		{"arm64 non-postgis unchanged", "docker.io/library/redis:7-alpine", "arm64", "docker.io/library/redis:7-alpine"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewriteArm64Image(tc.image, tc.goarch); got != tc.want {
				t.Fatalf("rewriteArm64Image(%q, %q) = %q, want %q", tc.image, tc.goarch, got, tc.want)
			}
		})
	}
}
