package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestProjectServiceForPicksTheEntryShape(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, svc := range []*config.CustomService{
		{Name: "mariadb-12-3", Image: "example.test/mariadb", Preset: "mariadb", PresetVersion: "12.3"},
		{Name: "handmade", Image: "example.test/handmade"},
	} {
		if err := config.SaveCustomService(svc); err != nil {
			t.Fatal(err)
		}
	}
	inline := &config.CustomService{Name: "teammate", Image: "example.test/teammate"}
	for _, c := range []struct {
		name, preset, version, image string
		inline                       *config.CustomService
	}{
		{name: "redis"},
		{name: "mariadb-12-3", preset: "mariadb", version: "12.3"},
		{name: "handmade", image: "example.test/handmade", inline: inline},
		{name: "teammate", image: "example.test/teammate", inline: inline},
		{name: "unknown"},
	} {
		got := ProjectServiceFor(c.name, c.inline)
		image := ""
		if got.Custom != nil {
			image = got.Custom.Image
		}
		if got.Name != c.name || got.Preset != c.preset || got.PresetVersion != c.version || image != c.image {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}

// The wizard writes a committed file, and an external server's host and user
// are this machine's, so only the name goes in.
func TestBuildProjectServicesKeepsAnExternalServerToItsName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := config.SaveCustomService(&config.CustomService{
		Name: "office", Family: "mysql", External: &config.ExternalDB{Host: "db.example.test", Port: 3306, User: "app"},
	}); err != nil {
		t.Fatal(err)
	}
	got := buildProjectServices([]string{"office"}, &config.ProjectConfig{})
	if len(got) != 1 || got[0].Name != "office" || got[0].Custom != nil || got[0].Preset != "" {
		t.Errorf("entry = %+v", got)
	}
}
