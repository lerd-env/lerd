package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectProjectRuntime(t *testing.T) {
	cases := []struct {
		name      string
		manifest  string
		wantLabel string
		wantFound bool
	}{
		{"node package.json", "package.json", "Node", true},
		{"node nvmrc only", ".nvmrc", "Node", true},
		{"go", "go.mod", "Go", true},
		{"python pyproject", "pyproject.toml", "Python", true},
		{"python requirements", "requirements.txt", "Python", true},
		{"python manage.py", "manage.py", "Python", true},
		{"rack", "config.ru", "Rack", true},
		{"ruby", "Gemfile", "Ruby", true},
		{"rust", "Cargo.toml", "Rust", true},
		{"empty dir", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.manifest != "" {
				if err := os.WriteFile(filepath.Join(dir, c.manifest), []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			rt, found := detectProjectRuntime(dir)
			label := ""
			if rt != nil {
				label = rt.label
			}
			if label != c.wantLabel || found != c.wantFound {
				t.Errorf("detectProjectRuntime(%s) = (%q, %v), want (%q, %v)",
					c.manifest, label, found, c.wantLabel, c.wantFound)
			}
		})
	}
}

// A Rails app with jsbundling ships a package.json next to config.ru; it is
// still a Rack app, not a Node one.
func TestDetectProjectRuntime_RackWinsOverPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "config.ru", "package.json")
	rt, found := detectProjectRuntime(dir)
	if !found || rt.label != "Rack" {
		t.Errorf("got %v, want Rack", rt)
	}
}

func TestDefaultDevCommand_Rack(t *testing.T) {
	rack := t.TempDir()
	writeFiles(t, rack, "config.ru", "Gemfile")
	if got, want := defaultDevCommand(rack), `sh -c 'exec bundle exec rackup --host "$HOST" --port "$PORT"'`; got != want {
		t.Errorf("rack: got %q, want %q", got, want)
	}

	rails := t.TempDir()
	writeFiles(t, rails, "config.ru", "Gemfile", "bin/rails")
	if got, want := defaultDevCommand(rails), `sh -c 'exec bin/rails server --binding "$HOST"'`; got != want {
		t.Errorf("rails: got %q, want %q", got, want)
	}
}

func TestIsRailsApp(t *testing.T) {
	rails := t.TempDir()
	writeFiles(t, rails, "config.ru", "bin/rails")
	if !isRailsApp(rails) {
		t.Error("config.ru + bin/rails should be a Rails app")
	}
	sinatra := t.TempDir()
	writeFiles(t, sinatra, "config.ru")
	if isRailsApp(sinatra) {
		t.Error("a bare Rack app is not Rails")
	}
}

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		path := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDefaultDevCommand(t *testing.T) {
	cases := []struct {
		manifest string
		want     string
	}{
		{"go.mod", "go run ."},
		{"requirements.txt", "python app.py"},
		{"manage.py", "python manage.py runserver"}, // Django: not python app.py
		{"Gemfile", "ruby app.rb"},
		{"Cargo.toml", "cargo run"},
		{"package.json", "npm run dev"},
		{"", ""}, // unknown runtime -> blank (proxy-only)
	}
	for _, c := range cases {
		dir := t.TempDir()
		if c.manifest != "" {
			if err := os.WriteFile(filepath.Join(dir, c.manifest), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if got := defaultDevCommand(dir); got != c.want {
			t.Errorf("defaultDevCommand(%q) = %q, want %q", c.manifest, got, c.want)
		}
	}
}

func TestStarterContainerfile(t *testing.T) {
	t.Run("runtime-specific base image, bind-mount model", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x"), 0644); err != nil {
			t.Fatal(err)
		}
		out := starterContainerfile(dir, 8080)
		if !strings.Contains(out, "FROM golang:") {
			t.Errorf("expected a Go base image, got:\n%s", out)
		}
		if !strings.Contains(out, "port 8080") {
			t.Errorf("expected the chosen port noted in the header, got:\n%s", out)
		}
		// Project is bind-mounted, so no instruction line should COPY the source
		// or set a WORKDIR (comment lines mentioning them are fine).
		for _, line := range strings.Split(out, "\n") {
			instr := strings.TrimSpace(line)
			if strings.HasPrefix(instr, "COPY") || strings.HasPrefix(instr, "WORKDIR") {
				t.Errorf("starter should not COPY/WORKDIR with a bind-mount, got line: %q", instr)
			}
		}
	})

	t.Run("rack starter serves config.ru on the chosen port", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, "config.ru")
		out := starterContainerfile(dir, 9393)
		if !strings.Contains(out, "FROM ruby:") || !strings.Contains(out, "rackup --host 0.0.0.0 --port 9393") {
			t.Errorf("expected a ruby image running rackup on 9393, got:\n%s", out)
		}
	})

	t.Run("unknown runtime falls back to a generic skeleton", func(t *testing.T) {
		out := starterContainerfile(t.TempDir(), 3000)
		if !strings.Contains(out, "FROM alpine:") {
			t.Errorf("expected a generic alpine base, got:\n%s", out)
		}
		if !strings.Contains(out, "port 3000") {
			t.Errorf("expected the chosen port noted in the header, got:\n%s", out)
		}
	})
}

func TestRubyShimDirs(t *testing.T) {
	home := t.TempDir()
	writeFiles(t, home, ".rbenv/shims/bundle", ".asdf/shims/bundle")
	site := t.TempDir()
	writeFiles(t, site, "Gemfile")
	got := rubyShimDirs(site, home)
	want := []string{filepath.Join(home, ".rbenv/shims"), filepath.Join(home, ".asdf/shims")}
	if strings.Join(got, ":") != strings.Join(want, ":") {
		t.Errorf("got %v, want %v", got, want)
	}
	if dirs := rubyShimDirs(t.TempDir(), home); dirs != nil {
		t.Errorf("non-Ruby site got %v, want none", dirs)
	}
}
