package podman

import (
	"strings"
	"testing"
)

// pdo_odbc picks the IBM DB2 backend unless unixODBC is named, and ext/odbc
// built statically turns every --with-<backend> on at once, so both need their
// own configure flags. Without them the image has no working odbc: DSN.
func TestPhpFpmContainerfile_BuildsODBCAgainstUnixODBC(t *testing.T) {
	tmpl, err := GetQuadletTemplate("lerd-php-fpm.Containerfile")
	if err != nil {
		t.Fatalf("read Containerfile: %v", err)
	}
	builder, runtime, ok := strings.Cut(tmpl, "# ── Runtime stage")
	if !ok {
		t.Fatal("runtime stage marker missing from Containerfile")
	}
	for _, want := range []string{
		"unixodbc-dev",
		"docker-php-ext-configure pdo_odbc --with-pdo-odbc=unixODBC,/usr",
		"AC_DEFUN([PHP_ALWAYS_SHARED],[])",
		"docker-php-ext-configure odbc --with-unixODBC=shared,/usr",
	} {
		if !strings.Contains(builder, want) {
			t.Errorf("builder stage must contain %q or the ODBC extensions build against the wrong driver manager", want)
		}
	}
	// libodbc.so.2 is dlopened at runtime; gcompat and libstdc++ are what lets
	// unixODBC load a vendor driver, which is always built against glibc.
	for _, want := range []string{"unixodbc", "gcompat", "libstdc++"} {
		if !strings.Contains(runtime, "\n        "+want+" \\\n") {
			t.Errorf("runtime stage must apk add %q or a registered ODBC driver never loads", want)
		}
	}
}

// The FrankenPHP image bakes the extensions through install-php-extensions,
// which brings unixODBC but not the glibc shim a vendor driver needs.
func TestFrankenPHPContainerfileShipsODBCRuntime(t *testing.T) {
	tmpl, err := GetQuadletTemplate("lerd-frankenphp.Containerfile")
	if err != nil {
		t.Fatalf("read Containerfile: %v", err)
	}
	for _, want := range []string{"unixodbc", "gcompat", "libstdc++"} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("FrankenPHP image must apk add %q so an ODBC driver loads there too", want)
		}
	}
}
