package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type provisionCalls struct {
	started   []string
	databases []string
	buckets   []string
}

// stubProvisionSeams stands in for the podman-backed start and create calls.
func stubProvisionSeams(t *testing.T, startErr error) *provisionCalls {
	t.Helper()
	calls := &provisionCalls{}
	prevRun, prevDB, prevBucket := ensureServiceRunning, createDatabase, createS3Bucket
	ensureServiceRunning = func(svc string) error {
		calls.started = append(calls.started, svc)
		return startErr
	}
	createDatabase = func(svc, name string) (bool, error) {
		calls.databases = append(calls.databases, svc+"/"+name)
		return true, nil
	}
	createS3Bucket = func(name string) (bool, error) {
		calls.buckets = append(calls.buckets, name)
		return true, nil
	}
	t.Cleanup(func() { ensureServiceRunning, createDatabase, createS3Bucket = prevRun, prevDB, prevBucket })
	return calls
}

func TestProvisionWiredService_CreatesTheSiteDatabaseAndItsTestingTwin(t *testing.T) {
	calls := stubProvisionSeams(t, nil)
	errs := provisionWiredService(t.TempDir(), "mysql", "shop", map[string]string{}, map[string]string{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := strings.Join(calls.databases, ","); got != "mysql/shop,mysql/shop_testing" {
		t.Errorf("databases = %q", got)
	}
}

func TestProvisionWiredService_ReportsADatabaseServiceThatWillNotStart(t *testing.T) {
	calls := stubProvisionSeams(t, errors.New("timeout"))
	errs := provisionWiredService(t.TempDir(), "postgres", "shop", map[string]string{}, map[string]string{})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "postgres did not start") {
		t.Fatalf("errs = %v", errs)
	}
	if len(calls.databases) != 0 {
		t.Errorf("created databases on a stopped service: %v", calls.databases)
	}
}

func TestProvisionWiredService_NamesTheBucketAfterTheSiteAndWiresItsURL(t *testing.T) {
	calls := stubProvisionSeams(t, nil)
	updates := map[string]string{}
	provisionWiredService(t.TempDir(), "rustfs", "my_shop", map[string]string{"AWS_BUCKET": "lerd"}, updates)
	if updates["AWS_BUCKET"] != "my-shop" || updates["AWS_URL"] != "http://localhost:9000/my-shop" {
		t.Errorf("updates = %v", updates)
	}
	if len(calls.buckets) != 1 || calls.buckets[0] != "my-shop" {
		t.Errorf("buckets = %v", calls.buckets)
	}
}

func TestProvisionWiredService_OnlyStartsAnyOtherService(t *testing.T) {
	calls := stubProvisionSeams(t, nil)
	provisionWiredService(t.TempDir(), "redis", "shop", map[string]string{}, map[string]string{})
	if len(calls.started) != 1 || len(calls.databases)+len(calls.buckets) != 0 {
		t.Errorf("calls = %+v", calls)
	}
}

// Tests that run on SQLite have no use for the testing twin (#2668).
func TestProvisionWiredService_SkipsTheTestingTwinWhenTestsRunOnSQLite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	calls := stubProvisionSeams(t, nil)
	dir := t.TempDir()
	phpunit := `<phpunit><php><env name="DB_CONNECTION" value="sqlite"/><env name="DB_DATABASE" value=":memory:"/></php></phpunit>`
	if err := os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(phpunit), 0o644); err != nil {
		t.Fatal(err)
	}
	updates := map[string]string{"DB_CONNECTION": "mysql", "DB_HOST": "lerd-mysql", "DB_DATABASE": "shop"}
	if errs := provisionWiredService(dir, "mysql", "shop", map[string]string{}, updates); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := strings.Join(calls.databases, ","); got != "mysql/shop" {
		t.Errorf("databases = %q, want only the site's own", got)
	}
}
