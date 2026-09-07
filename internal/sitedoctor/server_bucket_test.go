package sitedoctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

// s3Project writes a project whose env claims a bucket on lerd's rustfs. The
// endpoint line is what ties the claim to the local service: a value alone
// would also match a project pointed at real AWS.
func s3Project(t *testing.T, bucket string) string {
	t.Helper()
	dir := t.TempDir()
	env := "FILESYSTEM_DISK=s3\nAWS_BUCKET=" + bucket + "\nAWS_ENDPOINT=http://lerd-rustfs:9000\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckServerBucket_FailsWhenTheBucketIsNotThere(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := s3Project(t, "uploads")
	restore := stubEntityLister(func(string, *config.EntitySpec) ([]string, error) {
		return []string{"something-else"}, nil
	})
	defer restore()

	c, produced := checkServerBucket(dir)
	if !produced || c.Status != StatusFail {
		t.Fatalf("check = %+v (produced=%v), want a failure for the missing bucket", c, produced)
	}
	if !strings.Contains(c.Detail, `"uploads"`) {
		t.Errorf("detail should name the bucket, got %q", c.Detail)
	}
	if c.Fix != FixCreateBucket {
		t.Errorf("fix = %q, want %q", c.Fix, FixCreateBucket)
	}
}

func TestCheckServerBucket_PassesWhenTheBucketExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := s3Project(t, "uploads")
	restore := stubEntityLister(func(string, *config.EntitySpec) ([]string, error) {
		return []string{"uploads", "other"}, nil
	})
	defer restore()

	if c, produced := checkServerBucket(dir); !produced || c.Status != StatusOK {
		t.Fatalf("check = %+v (produced=%v), want ok", c, produced)
	}
}

// A service that cannot be reached leaves the site unjudged. Reporting the
// bucket as missing there would send the user to create one that may well exist.
func TestCheckServerBucket_UnreachableService_ProducesNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := s3Project(t, "uploads")
	restore := stubEntityLister(func(string, *config.EntitySpec) ([]string, error) {
		return nil, errors.New("container is not running")
	})
	defer restore()

	if c, produced := checkServerBucket(dir); produced {
		t.Fatalf("check = %+v, want nothing produced for an unreachable service", c)
	}
}

// A project pointed at real AWS names a bucket too. Without the reference test
// it would be measured against lerd's rustfs and reported as missing.
func TestCheckServerBucket_ProjectOnRealAWS_ProducesNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := t.TempDir()
	env := "FILESYSTEM_DISK=s3\nAWS_BUCKET=prod-assets\nAWS_DEFAULT_REGION=eu-west-1\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := stubEntityLister(func(string, *config.EntitySpec) ([]string, error) {
		return []string{"other"}, nil
	})
	defer restore()

	if c, produced := checkServerBucket(dir); produced {
		t.Fatalf("check = %+v, want nothing produced for a project not on lerd's service", c)
	}
}

// The fix re-checks inside the list cache's window, so the create has to drop
// the cached answer or the bucket it just made still reads as missing.
func TestForgetEntities_DropsTheCachedList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dir := s3Project(t, "uploads")
	names := []string{"other"}
	restore := stubEntityLister(func(string, *config.EntitySpec) ([]string, error) { return names, nil })
	defer restore()

	if c, _ := checkServerBucket(dir); c.Status != StatusFail {
		t.Fatalf("expected the first pass to fail, got %+v", c)
	}
	names = []string{"other", "uploads"}
	if c, _ := checkServerBucket(dir); c.Status != StatusFail {
		t.Fatalf("expected the cached list to still answer, got %+v", c)
	}
	ForgetEntities("rustfs")
	if c, _ := checkServerBucket(dir); c.Status != StatusOK {
		t.Fatalf("expected ok after the cache was dropped, got %+v", c)
	}
}
