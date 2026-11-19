package webserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Until a second backend exists, the current one is nginx, so a site applied
// through the seam lands in nginx's conf.d and removing it takes it back out.
func TestApplyAndRemoveSiteGoThroughNginx(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	site := config.Site{Name: "myapp", Domains: []string{"myapp.test"}, Path: "/srv/myapp", PHPVersion: "8.4"}
	conf := filepath.Join(config.NginxConfD(), "myapp.test.conf")

	if err := ApplySite(site); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(conf)
	if err != nil || !strings.Contains(string(data), "server_name myapp.test") {
		t.Fatalf("ApplySite did not write the nginx vhost: %v\n%s", err, data)
	}
	if err := RemoveSite(site.PrimaryDomain()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(conf); !os.IsNotExist(err) {
		t.Fatalf("RemoveSite left %s behind", conf)
	}
}
