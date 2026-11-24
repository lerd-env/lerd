package config

import (
	"os"
	"path/filepath"
	"testing"
)

func foreignDBProject(t *testing.T, marker string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files[marker] = ""
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestForeignDBHost(t *testing.T) {
	const lerdLogin = "DB_USERNAME=root\nDB_PASSWORD=lerd\n"
	const ownLogin = "DB_USERNAME=root\nDB_PASSWORD=\n"
	cases := []struct {
		name, marker string
		files        map[string]string
		want         string
	}{
		{"lerd container", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=lerd-mysql\nDB_PORT=3306\n" + ownLogin}, ""},
		{"remote server", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=db.example.com\n" + lerdLogin}, "db.example.com"},
		{"machine from a container", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=host.containers.internal\nDB_PORT=3306\n" + ownLogin}, "host.containers.internal:3306"},
		{"loopback server on its own port", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=33099\n" + ownLogin}, "127.0.0.1:33099"},
		// The port lerd-mysql would publish, held by somebody else's server.
		{"loopback server on lerd's default port", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\n" + ownLogin}, "127.0.0.1:3306"},
		{"lerd over loopback", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\n" + lerdLogin}, ""},
		// The port lerd just moved off still has to follow.
		{"lerd over a port it left", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3399\n" + lerdLogin}, ""},
		{"quoted lerd login", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=\"127.0.0.1\"\nDB_USERNAME=\"root\"\nDB_PASSWORD=\"lerd\"\n"}, ""},
		{"lerd with a personal login", "artisan", map[string]string{
			".env":               "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\nDB_USERNAME=me\nDB_PASSWORD=mine\n",
			".env.lerd_override": "DB_USERNAME=me\nDB_PASSWORD=mine\n"}, ""},
		{"lerd login before a personal one is written", "artisan", map[string]string{
			".env":               "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3399\n" + lerdLogin,
			".env.lerd_override": "DB_USERNAME=me\nDB_PASSWORD=mine\n"}, ""},
		{"override that names the server too", "artisan", map[string]string{
			".env":               "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\n" + ownLogin,
			".env.lerd_override": "DB_HOST=127.0.0.1\n" + ownLogin}, "127.0.0.1:3306"},
		{"override login for a server marked external", "artisan", map[string]string{
			".env":               "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\n" + ownLogin,
			".env.lerd_override": "LERD_EXTERNAL_SERVICES=mysql\n" + ownLogin}, "127.0.0.1:3306"},
		{"lerd postgres over loopback", "artisan", map[string]string{".env": "DB_CONNECTION=pgsql\nDB_HOST=127.0.0.1\nDB_PORT=5432\nDB_USERNAME=postgres\nDB_PASSWORD=lerd\n"}, ""},
		{"no host", "artisan", map[string]string{".env": "DB_CONNECTION=mysql\n"}, ""},
		{"no env", "artisan", map[string]string{}, ""},
		{"sqlite with a leftover host", "artisan", map[string]string{".env": "DB_CONNECTION=sqlite\nDB_HOST=127.0.0.1\n" + ownLogin}, ""},
		{"url on another server", "symfony.lock", map[string]string{".env.local": "DATABASE_URL=mysql://root:secret@127.0.0.1:3306/shop\n"}, "127.0.0.1:3306"},
		{"url on lerd over loopback", "symfony.lock", map[string]string{".env.local": "DATABASE_URL=\"mysql://root:lerd@127.0.0.1:3399/shop\"\n"}, ""},
		{"url on a lerd container", "symfony.lock", map[string]string{".env.local": "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop\n"}, ""},
		// The run seeds the env file from this one, so it is the connection kept.
		{"url only in the file the env is seeded from", "symfony.lock", map[string]string{".env": "DATABASE_URL=mysql://root:@127.0.0.1:3306/shop\n"}, "127.0.0.1:3306"},
		{"lerd url in the file the env is seeded from", "symfony.lock", map[string]string{".env": "DATABASE_URL=mysql://root:lerd@127.0.0.1:3306/shop\n"}, ""},
		{"env file wins over the one it is seeded from", "symfony.lock", map[string]string{".env": "DATABASE_URL=mysql://root:@127.0.0.1:3306/shop\n", ".env.local": "DATABASE_URL=mysql://root:lerd@lerd-mysql:3306/shop\n"}, ""},
		{"no framework", "notes.txt", map[string]string{".env": "DB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3306\n" + ownLogin}, "127.0.0.1:3306"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			got, foreign := ForeignDBHost(foreignDBProject(t, c.marker, c.files))
			if got != c.want || foreign != (c.want != "") {
				t.Errorf("ForeignDBHost = %q, %v; want %q", got, foreign, c.want)
			}
		})
	}
}

// A definition that declares no login leaves only the published port to go by.
func TestForeignDBHost_WithoutADeclaredLogin(t *testing.T) {
	for env, want := range map[string]bool{
		"DB_HOST=127.0.0.1\n":                  false,
		"DB_HOST=127.0.0.1\nDB_PORT=3306\n":    false,
		"DB_HOST=127.0.0.1\nDB_PORT=33099\n":   true,
		"DB_HOST=db.example.com\nDB_PORT=33\n": true,
	} {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("XDG_DATA_HOME", t.TempDir())
		fw := &Framework{Name: "bare", Label: "Bare", Env: FrameworkEnvConf{File: ".env", Services: map[string]FrameworkServiceDef{
			"mysql": {Vars: []string{"DB_HOST=lerd-mysql", "DB_DATABASE={{site}}"}},
		}}}
		if err := SaveFramework(fw); err != nil {
			t.Fatal(err)
		}
		dir := foreignDBProject(t, ".lerd.yaml", map[string]string{".env": env})
		if err := os.WriteFile(filepath.Join(dir, ".lerd.yaml"), []byte("framework: bare\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, got := ForeignDBHost(dir); got != want {
			t.Errorf("%q foreign = %v, want %v", env, got, want)
		}
	}
}
