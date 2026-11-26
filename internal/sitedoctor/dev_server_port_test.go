package sitedoctor

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestCheckDevServerPort(t *testing.T) {
	movable := &config.Framework{DevServer: &config.FrameworkDevServer{MovePort: true}}
	cases := map[string]struct {
		status string
		free   bool
		fw     *config.Framework
		want   string
		detail string
	}{
		"running":          {"active", false, nil, StatusOK, ""},
		"stopped and free": {"inactive", true, nil, StatusOK, ""},
		"taken":            {"activating", false, nil, StatusFail, "lerd link"},
		"taken, movable":   {"inactive", false, movable, StatusFail, "lerd restart"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			dir := t.TempDir()
			if err := config.AddSite(config.Site{Name: "api", Domains: []string{"api.test"}, Path: dir, HostPort: 43000, HostCommand: "npm run dev"}); err != nil {
				t.Fatal(err)
			}
			pf, ps := devServerPortFree, devServerStatus
			t.Cleanup(func() { devServerPortFree, devServerStatus = pf, ps })
			devServerPortFree = func(int) bool { return c.free }
			devServerStatus = func(string) (string, error) { return c.status, nil }
			got, ok := checkDevServerPort(dir, c.fw)
			if !ok || string(got.Status) != string(c.want) || !strings.Contains(got.Detail, c.detail) {
				t.Errorf("check = %+v (%v)", got, ok)
			}
		})
	}
	if _, ok := checkDevServerPort(t.TempDir(), nil); ok {
		t.Error("a directory that is no site got a port check")
	}
}
