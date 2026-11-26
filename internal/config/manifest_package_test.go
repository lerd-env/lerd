package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestPackage(t *testing.T) {
	cases := map[string]struct {
		file, body, name string
		want             bool
	}{
		"npm dependency":         {"package.json", `{"dependencies":{"@acme/core":"^1"}}`, "@acme/core", true},
		"npm dev dependency":     {"package.json", `{"devDependencies":{"@acme/core":"^1"}}`, "@acme/core", true},
		"npm peer only":          {"package.json", `{"peerDependencies":{"@acme/core":"^1"}}`, "@acme/core", false},
		"npm absent":             {"package.json", `{"dependencies":{"other":"1"}}`, "@acme/core", false},
		"npm malformed":          {"package.json", `{"dependencies":`, "@acme/core", false},
		"requirements pinned":    {"requirements.txt", "# web\nDjango==5.2\n", "django", true},
		"requirements extras":    {"requirements.txt", "django[argon2]>=5 ; python_version>'3'\r\n", "django", true},
		"requirements other":     {"requirements.txt", "django-environ==1\n", "django", false},
		"requirements comment":   {"requirements.txt", "# django\n-r django\n", "django", false},
		"requirements separator": {"requirements.txt", "Flask_SQLAlchemy\n", "flask-sqlalchemy", true},
		"pyproject":              {"pyproject.toml", "[project]\ndependencies = [\"Django>=5\"]\n", "django", true},
		"pyproject group":        {"pyproject.toml", "[dependency-groups]\ndev = [\"django\", {include-group = \"x\"}]\nx = []\n", "django", true},
		"poetry":                 {"pyproject.toml", "[tool.poetry.dependencies]\npython = \"^3\"\nDjango = {version = \"5\"}\n", "django", true},
		"pyproject malformed":    {"pyproject.toml", "[project", "django", false},
		"gem":                    {"Gemfile", "source 'x'\n  gem \"rails\", '~> 8'\n", "rails", true},
		"gem other":              {"Gemfile", "gem 'rails-i18n'\n# gem 'rails'\n", "rails", false},
		"cargo":                  {"Cargo.toml", "[dependencies]\naxum = { version = \"0.8\" }\n", "axum", true},
		"cargo absent":           {"Cargo.toml", "[package]\nname = \"axum\"\n", "axum", false},
		"unknown manifest":       {"mix.exs", "phoenix", "phoenix", false},
		"empty name":             {"Gemfile", "gem 'rails'", "", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.file), []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			rule := FrameworkRule{Package: &ManifestPackage{File: c.file, Name: c.name}}
			if got := MatchesDetectRule(dir, rule); got != c.want {
				t.Errorf("detect = %v, want %v", got, c.want)
			}
		})
	}
	t.Run("no manifest", func(t *testing.T) {
		if MatchesDetectRule(t.TempDir(), FrameworkRule{Package: &ManifestPackage{File: "Gemfile", Name: "rails"}}) {
			t.Error("matched without a manifest")
		}
	})
}

// A PHP app may carry the package for its frontend: still a PHP project, though
// a worker gated on the package may run in it.
func TestManifestPackage_composerProjectStaysPHP(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"package.json":  `{"dependencies":{"@acme/core":"^1"}}`,
		"composer.json": `{"require":{"php":"^8.3"}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rule := FrameworkRule{Package: &ManifestPackage{File: "package.json", Name: "@acme/core"}}
	if MatchesDetectRule(dir, rule) {
		t.Error("a composer project was detected through its package.json")
	}
	if !MatchesRule(dir, rule) {
		t.Error("a gate on the package should hold in a composer project")
	}
}

func TestManifestPackage_bindsFromTheStoreIndex(t *testing.T) {
	var r FrameworkRule
	if err := json.Unmarshal([]byte(`{"package":{"file":"Gemfile","name":"rails"}}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Package == nil || r.Package.File != "Gemfile" || r.Package.Name != "rails" {
		t.Errorf("package = %+v", r.Package)
	}
}

func TestSanitizeProjectFrameworkDef_dropsTheDevServer(t *testing.T) {
	safe := SanitizeProjectFrameworkDef(&Framework{Name: "x", DevServer: &FrameworkDevServer{Command: "evil"}})
	if safe.DevServer != nil {
		t.Error("an untrusted definition kept its dev server command")
	}
}
