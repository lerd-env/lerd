package serviceops

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/podman"
)

// databaseNamePattern is the strict shape an entity name must have to reach a
// path or SQL sink: it must start with a letter, digit or underscore and carry
// only those plus dashes and interior dots (S3 bucket names carry dots), which
// covers every name lerd generates while excluding path separators, leading-dot
// segments like ".." and every shell and SQL metacharacter.
var databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

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
// not already exist, through the create action the engine's preset declares.
// Returns (true, nil) if created, (false, nil) if it already existed or the
// engine declares no create action, or (false, err) on failure.
func CreateDatabase(svc, name string) (bool, error) {
	if err := ValidateDatabaseName(name); err != nil {
		return false, err
	}
	spec := EntityFor(svc, "databases")
	if _, ok := entityAction(spec, "create"); !ok {
		return false, nil
	}
	exists, err := EntityExists(svc, spec, name)
	if err != nil {
		return false, err
	}
	if exists {
		// The engine's up-front extensions belong to every database it holds, so a
		// site created before its engine declared an extension picks it up on the
		// next run rather than only ever on a new database.
		return false, EnsureExtensions(svc, name)
	}
	if err := RunEntityAction(svc, spec, "create", name); err != nil {
		return false, err
	}
	return true, EnsureExtensions(svc, name)
}

// DropDatabase removes the named database from the service container through
// the declared drop action. Returns (true, nil) if it was dropped, (false, nil)
// if it was already gone or the engine declares no drop action, or
// (false, err) on failure.
func DropDatabase(svc, name string) (bool, error) {
	if err := ValidateDatabaseName(name); err != nil {
		return false, err
	}
	spec := EntityFor(svc, "databases")
	if _, ok := entityAction(spec, "drop"); !ok {
		return false, nil
	}
	exists, err := EntityExists(svc, spec, name)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if err := RunEntityAction(svc, spec, "drop", name); err != nil {
		return false, err
	}
	return true, nil
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
