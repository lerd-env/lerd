package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pluginPresetYAML = "name: queuebox\ndescription: A queue\nimage: docker.io/library/queuebox:1\n"

const pluginFrameworkYAML = "name: acmefw\nlabel: Acme\npublic_dir: web\ndetect:\n  - file: acme.lock\n"

// pluginDefinitions lays out one plugin shipping the given files under its
// folder and points the plugin layer at it.
func pluginDefinitions(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "acme")
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	orig := PluginDefinitionDirs
	PluginDefinitionDirs = func(kind string) []string { return []string{filepath.Join(dir, kind)} }
	t.Cleanup(func() { PluginDefinitionDirs = orig })
	return dir
}

func TestPluginPreset_IsListedAndLoads(t *testing.T) {
	storeSandbox(t)
	pluginDefinitions(t, map[string]string{"services/queuebox.yaml": pluginPresetYAML})

	if !PresetExists("queuebox") {
		t.Fatal("a preset a plugin ships must exist")
	}
	p, err := LoadPreset("queuebox")
	if err != nil || p.Image != "docker.io/library/queuebox:1" {
		t.Fatalf("LoadPreset = %+v, %v", p, err)
	}
	metas, _ := ListPresets()
	found := false
	for _, m := range metas {
		found = found || m.Name == "queuebox"
	}
	if !found {
		t.Error("the service picker must list a plugin's preset")
	}
}

// The store and lerd's own presets come first, so a name the store publishes
// later stays the store's.
func TestPluginPreset_StoreWins(t *testing.T) {
	storeSandbox(t)
	pluginDefinitions(t, map[string]string{"services/mysql.yaml": "name: mysql\nimage: docker.io/evil/mysql:1\n"})
	p, err := LoadPreset("mysql")
	if err != nil || strings.Contains(p.Image, "evil") {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestPluginFramework_ResolvesDetectsAndLists(t *testing.T) {
	storeSandbox(t)
	pluginDefinitions(t, map[string]string{"frameworks/acmefw.yaml": pluginFrameworkYAML})
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "acme.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if name, ok := DetectFramework(project); !ok || name != "acmefw" {
		t.Errorf("DetectFramework = %q, %v", name, ok)
	}
	if fw, ok := GetFramework("acmefw"); !ok || fw.PublicDir != "web" {
		t.Errorf("GetFramework = %+v, %v", fw, ok)
	}
	if fw, ok := GetFrameworkForDir("acmefw", project); !ok || fw.PublicDir != "web" {
		t.Errorf("GetFrameworkForDir = %+v, %v", fw, ok)
	}
	if src := GetFrameworkSource("acmefw"); src != SourcePlugin {
		t.Errorf("source = %q, want %q", src, SourcePlugin)
	}
	listed := false
	for _, fw := range ListFrameworks() {
		listed = listed || fw.Name == "acmefw"
	}
	detailed := false
	for _, info := range ListFrameworksDetailed() {
		detailed = detailed || info.Name == "acmefw" && info.Source == SourcePlugin
	}
	if !listed || !detailed {
		t.Errorf("listed=%v detailed=%v, want the plugin framework in both", listed, detailed)
	}
}

func TestPluginFramework_UserDefinitionWins(t *testing.T) {
	storeSandbox(t)
	pluginDefinitions(t, map[string]string{"frameworks/acmefw.yaml": pluginFrameworkYAML})
	if err := os.MkdirAll(FrameworksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(FrameworksDir(), "acmefw.yaml"), []byte("name: acmefw\nlabel: Mine\npublic_dir: mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fw, ok := GetFramework("acmefw"); !ok || fw.PublicDir != "mine" {
		t.Errorf("GetFramework = %+v, %v", fw, ok)
	}
}

func TestCheckPluginDefinitions(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"valid", map[string]string{"services/queuebox.yaml": pluginPresetYAML, "frameworks/acmefw.yaml": pluginFrameworkYAML}, ""},
		{"preset name differs from file", map[string]string{"services/other.yaml": pluginPresetYAML}, "other"},
		{"preset without image", map[string]string{"services/queuebox.yaml": "name: queuebox\n"}, "image"},
		{"framework name differs from file", map[string]string{"frameworks/other.yaml": pluginFrameworkYAML}, "other"},
		{"versioned framework file", map[string]string{"frameworks/acmefw@2.yaml": pluginFrameworkYAML}, "acmefw@2"},
		{"framework that does not parse", map[string]string{"frameworks/acmefw.yaml": "name: [\n"}, "acmefw"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := pluginDefinitions(t, c.files)
			err := CheckPluginDefinitions(dir)
			if c.want == "" {
				if err != nil {
					t.Fatalf("unexpected %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to name %q", err, c.want)
			}
		})
	}
}

// A plugin may not take a name lerd already has from anywhere else, so what
// it ships never silently changes what an existing site resolves to.
func TestPluginDefinitionClashes(t *testing.T) {
	storeSandbox(t)
	dir := pluginDefinitions(t, map[string]string{
		"services/mysql.yaml":     "name: mysql\nimage: docker.io/x/mysql:1\n",
		"services/queuebox.yaml":  pluginPresetYAML,
		"frameworks/laravel.yaml": "name: laravel\nlabel: L\n",
		"frameworks/acmefw.yaml":  pluginFrameworkYAML,
	})
	// The plugin layer above already serves dir, so pretend it is not installed.
	PluginDefinitionDirs = func(string) []string { return nil }

	got := PluginDefinitionClashes(dir, "")
	if strings.Join(got, ",") != "service mysql,framework laravel" {
		t.Errorf("clashes = %v", got)
	}
}

// Updating a plugin re-ships its own names, which are not clashes.
func TestPluginDefinitionClashes_OwnInstalledCopyIsNotAClash(t *testing.T) {
	storeSandbox(t)
	installed := pluginDefinitions(t, map[string]string{"services/queuebox.yaml": pluginPresetYAML, "frameworks/acmefw.yaml": pluginFrameworkYAML})
	staged := filepath.Join(t.TempDir(), "acme")
	if err := os.CopyFS(staged, os.DirFS(installed)); err != nil {
		t.Fatal(err)
	}
	if got := PluginDefinitionClashes(staged, installed); len(got) != 0 {
		t.Errorf("clashes = %v, want none", got)
	}
	if got := PluginDefinitionClashes(staged, ""); len(got) != 2 {
		t.Errorf("a second plugin shipping the same names clashes with the first, got %v", got)
	}
}
