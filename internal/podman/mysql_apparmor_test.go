package podman

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// Ubuntu's mysql-server AppArmor profile attaches by path to /usr/sbin/mysqld,
// also in lerd's rootless container (config ignored, wrong socket, no clean
// stop), so every version must start mysqld from a path the profile misses.
func TestMySQLStartsOutsideTheHostAppArmorPath(t *testing.T) {
	preset, err := config.LoadPreset("mysql")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range preset.Versions {
		svc, err := preset.Resolve(v.Tag)
		if err != nil {
			t.Fatalf("resolve %s: %v", v.Tag, err)
		}
		out := GenerateCustomQuadlet(svc)
		want := `Exec=sh -c "cp -f /usr/sbin/mysqld /usr/local/sbin/mysqld && exec docker-entrypoint.sh mysqld --basedir=/usr"`
		if !strings.Contains(out, want) {
			t.Errorf("mysql %s quadlet does not move mysqld off /usr/sbin, got:\n%s", v.Tag, out)
		}
	}
}
