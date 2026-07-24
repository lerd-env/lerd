package serviceops

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// escapeIdentBacktick makes name safe inside a MySQL/MariaDB `...` identifier
// by doubling embedded backticks; escapeIdentDQuote does the same for a
// PostgreSQL "..." identifier, and escapeSQLLiteral for a '...' string literal.
// Callers already pass slugged names, but these guard the SQL sinks directly so
// a name that ever reaches here unsanitised cannot break out of its quoting.
func escapeIdentBacktick(name string) string { return strings.ReplaceAll(name, "`", "``") }

// postgresDropSQL drops a database in one statement, closing whatever still has
// it open. Terminating the sessions first and dropping after loses a race with
// anything that reconnects, which on a dev machine is the site's own pool.
func postgresDropSQL(name string) string {
	return fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE);`, escapeIdentDQuote(name))
}
func escapeIdentDQuote(name string) string { return strings.ReplaceAll(name, `"`, `""`) }
func escapeSQLLiteral(v string) string     { return strings.ReplaceAll(v, "'", "''") }

// escapeMySQLLiteral is escapeSQLLiteral for MySQL and MariaDB, which treat a
// backslash as an escape character unless NO_BACKSLASH_ESCAPES is set. Doubling
// the backslash first stops `\'` from escaping the doubled quote and ending the
// literal. PostgreSQL keeps escapeSQLLiteral: standard_conforming_strings makes
// a backslash ordinary, so doubling it there would corrupt the value.
func escapeMySQLLiteral(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), "'", "''")
}

// databaseNamePattern is the strict shape a database name must have to reach a
// path or SQL sink: it must start with a letter, digit or underscore and carry
// only those plus dashes, which covers every name lerd generates while excluding
// path separators, dot segments and every SQL metacharacter.
var databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

// maxDatabaseNameLen is MySQL's identifier limit, the tighter of the two engines.
const maxDatabaseNameLen = 64

// ValidateDatabaseName guards the sinks that assume a slugged name: the snapshot
// paths built with filepath.Join and the information_schema lookups built by
// string interpolation.
func ValidateDatabaseName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("a database name is required")
	case len(name) > maxDatabaseNameLen:
		return fmt.Errorf("database name is longer than %d characters", maxDatabaseNameLen)
	case !databaseNamePattern.MatchString(name):
		return fmt.Errorf("invalid database name %q: use letters, digits, underscores and dashes only", name)
	}
	return nil
}

// CreateDatabase creates dbName inside the named service container if it does
// not already exist. svc is the service name (e.g. "mysql", "mysql-5-6",
// "mariadb-11", "postgres-14"); the container is always "lerd-<svc>". The
// SQL client used is determined by the family inferred from svc.
// Returns (true, nil) if created, (false, nil) if it already existed,
// or (false, err) on failure.
func CreateDatabase(svc, name string) (bool, error) {
	container := "lerd-" + svc
	family := svc
	if inferred := config.FamilyOfName(svc); inferred != "" {
		family = inferred
	}
	switch family {
	case "mysql", "mariadb":
		binaries := []string{"mysql", "mariadb"}
		if family == "mariadb" {
			binaries = []string{"mariadb", "mysql"}
		}
		var lastErr error
		for _, bin := range binaries {
			check := podman.Cmd("exec", container, bin, "-uroot", "-plerd",
				"-sNe", fmt.Sprintf("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='%s';", escapeMySQLLiteral(name)))
			out, err := check.Output()
			if err != nil {
				lastErr = err
				continue
			}
			if strings.TrimSpace(string(out)) != "0" {
				return false, nil
			}
			cmd := podman.Cmd("exec", container, bin, "-uroot", "-plerd",
				"-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", escapeIdentBacktick(name)))
			// Capture stderr rather than inheriting it: mysql prints a noisy
			// "[Warning] Using a password on the command line interface" that would
			// otherwise clobber the live "configuring .env" spinner. Surface it only
			// on a real failure.
			if out, err := cmd.CombinedOutput(); err != nil {
				return false, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			}
			return true, nil
		}
		return false, lastErr
	case "postgres":
		cmd := podman.Cmd("exec", container, "psql", "-U", "postgres",
			"-c", fmt.Sprintf(`CREATE DATABASE "%s";`, escapeIdentDQuote(name)))
		out, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(out), "already exists") {
				// Applied to a database that was already there too, so a site created
				// before its engine declared an extension picks it up on the next run
				// rather than only ever on a new database.
				return false, EnsureExtensions(svc, name)
			}
			return false, fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		// The engine's up-front extensions belong to every database it holds, so a
		// dropped and recreated one comes back whole rather than missing what the
		// site was built on.
		return true, EnsureExtensions(svc, name)
	default:
		return false, nil
	}
}

// DropDatabase removes the named database from the service container. Returns
// (true, nil) if it was dropped, (false, nil) if it was already gone, or
// (false, err) on failure.
func DropDatabase(svc, name string) (bool, error) {
	container := "lerd-" + svc
	family := svc
	if inferred := config.FamilyOfName(svc); inferred != "" {
		family = inferred
	}
	switch family {
	case "mysql", "mariadb":
		binaries := []string{"mysql", "mariadb"}
		if family == "mariadb" {
			binaries = []string{"mariadb", "mysql"}
		}
		var lastErr error
		for _, bin := range binaries {
			check := podman.Cmd("exec", container, bin, "-uroot", "-plerd",
				"-sNe", fmt.Sprintf("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='%s';", escapeMySQLLiteral(name)))
			out, err := check.Output()
			if err != nil {
				lastErr = err
				continue
			}
			if strings.TrimSpace(string(out)) == "0" {
				return false, nil
			}
			cmd := podman.Cmd("exec", container, bin, "-uroot", "-plerd",
				"-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", escapeIdentBacktick(name)))
			cmd.Stderr = os.Stderr
			return true, cmd.Run()
		}
		return false, lastErr
	case "postgres":
		cmd := podman.Cmd("exec", container, "psql", "-U", "postgres", "-c", postgresDropSQL(name))
		out, err := cmd.CombinedOutput()
		if err != nil && strings.Contains(string(out), "syntax error") {
			// WITH (FORCE) needs postgres 13. On an older server, close the sessions
			// and drop in two steps, which is all that was ever available there.
			_ = podman.Cmd("exec", container, "psql", "-U", "postgres",
				"-c", fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();`, escapeSQLLiteral(name))).Run()
			cmd = podman.Cmd("exec", container, "psql", "-U", "postgres",
				"-c", fmt.Sprintf(`DROP DATABASE IF EXISTS "%s";`, escapeIdentDQuote(name)))
			out, err = cmd.CombinedOutput()
		}
		if err != nil {
			if strings.Contains(string(out), "does not exist") {
				return false, nil
			}
			return false, fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return true, nil
	default:
		return false, nil
	}
}

// S3BucketName converts a project handle into a valid S3 bucket name:
// lowercase, hyphens instead of underscores, leading/trailing non-alphanumerics
// stripped, max length 63.
func S3BucketName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.':
			b.WriteRune(r)
		case r == '_', r == '-', r == ' ':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if len(out) > 63 {
		out = out[:63]
	}
	if out == "" {
		out = "lerd"
	}
	return out
}

// EnsureS3Bucket creates a bucket for the given name in lerd-rustfs using an
// ephemeral mc container. Returns (true, nil) if created, (false, nil) if it
// already existed, or (false, err) on failure. Retries up to 3 times (2s apart)
// to bridge the window between the host TCP port becoming reachable and the
// container network being fully ready for mc operations.
func EnsureS3Bucket(name string) (bool, error) {
	const (
		alias   = "lerd"
		mcImage = "docker.io/minio/mc:latest"
		mcEnv   = "MC_HOST_lerd=http://lerd:lerdpassword@lerd-rustfs:9000"
	)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		lsCmd := podman.Cmd("run", "--rm", "--network", "lerd",
			"-e", mcEnv, mcImage, "ls", alias+"/"+name)
		if lsCmd.Run() == nil {
			return false, nil
		}

		mbCmd := podman.Cmd("run", "--rm", "--network", "lerd",
			"-e", mcEnv, mcImage, "mb", alias+"/"+name)
		out, err := mbCmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("%s", strings.TrimSpace(string(out)))
			continue
		}

		pubCmd := podman.Cmd("run", "--rm", "--network", "lerd",
			"-e", mcEnv, mcImage, "anonymous", "set", "public", alias+"/"+name)
		if out, err := pubCmd.CombinedOutput(); err != nil {
			return false, fmt.Errorf("mc anonymous set public: %s", strings.TrimSpace(string(out)))
		}
		return true, nil
	}
	return false, lastErr
}
