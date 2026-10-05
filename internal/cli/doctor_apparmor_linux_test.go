//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func apparmorRoot(t *testing.T, enabled string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if enabled != "" {
		files["sys/module/apparmor/parameters/enabled"] = enabled
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// A native MySQL or MariaDB server's profile attaches by path to the mysqld in
// lerd's rootless container too, so only an enforced one is reported.
func TestEnforcedMysqldProfiles(t *testing.T) {
	enforce := "/usr/sbin/mysqld {\n  /etc/mysql/** r,\n}\n"
	cases := []struct {
		name    string
		enabled string
		files   map[string]string
		want    []string
	}{
		{"mysql-server profile enforced", "Y\n", map[string]string{"etc/apparmor.d/usr.sbin.mysqld": enforce}, []string{"/etc/apparmor.d/usr.sbin.mysqld"}},
		{"mariadb profile enforced", "Y\n", map[string]string{"etc/apparmor.d/usr.sbin.mariadbd": enforce}, []string{"/etc/apparmor.d/usr.sbin.mariadbd"}},
		{"disabled through apparmor.d/disable", "Y\n", map[string]string{"etc/apparmor.d/usr.sbin.mysqld": enforce, "etc/apparmor.d/disable/usr.sbin.mysqld": ""}, nil},
		{"complain mode denies nothing", "Y\n", map[string]string{"etc/apparmor.d/usr.sbin.mysqld": "/usr/sbin/mysqld flags=(complain) {\n}\n"}, nil},
		{"AppArmor off in the kernel", "N\n", map[string]string{"etc/apparmor.d/usr.sbin.mysqld": enforce}, nil},
		{"no AppArmor at all", "", map[string]string{"etc/apparmor.d/usr.sbin.mysqld": enforce}, nil},
		{"no profile", "Y\n", map[string]string{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enforcedMysqldProfiles(apparmorRoot(t, c.enabled, c.files))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
