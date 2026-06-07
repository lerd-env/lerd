package cli

import (
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func TestValidateMovePair(t *testing.T) {
	cases := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{"postgres to alternate", "postgres", "postgres-18", false},
		{"alternate to canonical", "postgres-17", "postgres", false},
		{"mysql to alternate", "mysql", "mysql-5-7", false},
		{"same service", "postgres", "postgres", true},
		{"cross family", "postgres", "mysql", true},
		{"mysql to mariadb is cross family", "mysql", "mariadb", true},
		{"sqlite source unsupported", "sqlite", "postgres", true},
		{"empty source", "", "postgres", true},
		{"empty target", "postgres", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMovePair(tc.from, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("validateMovePair(%q,%q) = nil, want error", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateMovePair(%q,%q) = %v, want nil", tc.from, tc.to, err)
			}
		})
	}
}

func TestSiteDBService(t *testing.T) {
	t.Run("explicit services entry wins", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".lerd.yaml", "services:\n  - postgres-18\n")
		writeFile(t, dir, ".env", "DB_HOST=lerd-postgres\n")
		if got := siteDBService(dir); got != "postgres-18" {
			t.Fatalf("siteDBService = %q, want postgres-18", got)
		}
	})

	t.Run("db block service", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".lerd.yaml", "db:\n  service: postgres-17\n")
		if got := siteDBService(dir); got != "postgres-17" {
			t.Fatalf("siteDBService = %q, want postgres-17", got)
		}
	})

	t.Run("falls back to .env DB_HOST", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".env", "DB_HOST=lerd-mysql-5-7\n")
		if got := siteDBService(dir); got != "mysql-5-7" {
			t.Fatalf("siteDBService = %q, want mysql-5-7", got)
		}
	})

	t.Run("sqlite returns empty", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".lerd.yaml", "services:\n  - sqlite\n")
		if got := siteDBService(dir); got != "" {
			t.Fatalf("siteDBService = %q, want empty", got)
		}
	})

	t.Run("nothing configured returns empty", func(t *testing.T) {
		dir := t.TempDir()
		if got := siteDBService(dir); got != "" {
			t.Fatalf("siteDBService = %q, want empty", got)
		}
	})
}

func TestResolveMoveSites(t *testing.T) {
	shop := t.TempDir()
	writeFile(t, shop, ".lerd.yaml", "services:\n  - postgres\n")
	blog := t.TempDir()
	writeFile(t, blog, ".lerd.yaml", "services:\n  - postgres\n")
	api := t.TempDir()
	writeFile(t, api, ".lerd.yaml", "services:\n  - mysql\n")

	reg := &config.SiteRegistry{Sites: []config.Site{
		{Name: "shop", Path: shop},
		{Name: "blog", Path: blog},
		{Name: "api", Path: api},
	}}

	t.Run("all picks every site on source", func(t *testing.T) {
		got, err := resolveMoveSites(reg, "postgres", nil, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d sites, want 2 (shop, blog)", len(got))
		}
	})

	t.Run("named sites pass when on source", func(t *testing.T) {
		got, err := resolveMoveSites(reg, "postgres", []string{"shop"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Name != "shop" {
			t.Fatalf("got %+v, want [shop]", got)
		}
	})

	t.Run("named site on a different service errors", func(t *testing.T) {
		if _, err := resolveMoveSites(reg, "postgres", []string{"api"}, false); err == nil {
			t.Fatal("expected error moving a mysql site from postgres")
		}
	})

	t.Run("unknown site errors", func(t *testing.T) {
		if _, err := resolveMoveSites(reg, "postgres", []string{"nope"}, false); err == nil {
			t.Fatal("expected error for unknown site")
		}
	})
}
