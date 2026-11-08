package config

import "testing"

// A domain becomes a vhost file name and its server_name, and a project's
// .lerd.yaml, the dashboard and MCP all supply one, so the registry is where a
// path or a directive smuggled into it must be stopped.
func TestAddSite_refusesADomainThatCouldLeaveConfDOrInjectNginx(t *testing.T) {
	setDataDir(t)
	for _, d := range []string{
		"../../evil.test",
		`..\evil.test`,
		"a;include /etc/passwd.test",
		"a{}.test",
		"a b.test",
		"a\nserver_name x.test",
	} {
		if err := AddSite(Site{Name: "app", Path: t.TempDir(), Domains: []string{d}}); err == nil {
			t.Errorf("AddSite accepted domain %q", d)
		}
	}
	if err := AddSite(Site{Name: "app", Path: t.TempDir(), Domains: []string{"shop.app.test"}}); err != nil {
		t.Fatalf("AddSite refused a plain domain: %v", err)
	}
}
