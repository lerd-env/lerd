package serviceops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// ValidateDatabaseName guards the sinks that assume a slugged name; the rule
// lives in config so .lerd.yaml can refuse a bad db.database as it loads.
func ValidateDatabaseName(name string) error { return config.ValidateDatabaseName(name) }

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

// EnsureS3Bucket creates a bucket for the given name on the S3 service.
// Returns (true, nil) if created, (false, nil) if it already existed, or
// (false, err) on failure. Retries up to 3 times (2s apart) to bridge the
// window between the host port becoming reachable and the service answering
// on it.
func EnsureS3Bucket(name string) (bool, error) {
	if err := ValidateDatabaseName(name); err != nil {
		return false, err
	}
	spec := EntityFor(s3Service, "buckets")
	if spec == nil || spec.Driver != s3Driver {
		return false, fmt.Errorf("%s declares no S3 buckets", s3Service)
	}
	c, err := s3ClientFor(s3Service, spec.Env)
	if err != nil {
		return false, err
	}

	// One attempt: the existence check and the create share a deadline, and a
	// failure of either is what the retry is for.
	attempt := func() (bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), s3Timeout)
		defer cancel()
		exists, err := c.BucketExists(ctx, name)
		if err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
		return true, s3CreateBucket(ctx, c, name)
	}

	var lastErr error
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		created, err := attempt()
		if err == nil {
			return created, nil
		}
		lastErr = err
	}
	return false, lastErr
}
