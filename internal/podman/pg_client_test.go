package podman

import (
	"strings"
	"testing"
)

// Framework commands shell out to pg_dump/psql inside the PHP container
// (artisan schema:dump, schema:load), so the runtime stage must ship them.
// pg_dump refuses a newer server, so the newest client the base offers wins.
func TestPHPImageShipsNewestPostgresClient(t *testing.T) {
	tmpl, err := GetQuadletTemplate("lerd-php-fpm.Containerfile")
	if err != nil {
		t.Fatalf("read Containerfile: %v", err)
	}
	runtime := tmpl[strings.Index(tmpl, "# ── Runtime stage"):]
	if !strings.Contains(runtime, "for v in 18 17 16 15 14; do apk search -qe postgresql${v}-client") {
		t.Error("PHP-FPM runtime stage must pick the newest postgresqlNN-client the index lists, 18 first")
	}
	if !strings.Contains(runtime, "apk add --no-cache postgresql${pg}-client") {
		t.Error("PHP-FPM runtime stage must install the picked postgres client")
	}
}
