package serviceops

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
)

// reprovDB and reprovBucket are the seams ReprovisionLinkedSites uses to talk
// to the running service. They default to the real implementations and are
// swapped in tests.
var (
	reprovDB     = CreateDatabase
	reprovBucket = EnsureS3Bucket
)

// familyHoldsSiteState reports whether a service family owns state that belongs
// to an individual site: a database for the db engines, a bucket for object
// storage. Cache and search families hold nothing a site expects to find again.
func familyHoldsSiteState(family string) bool {
	switch family {
	case "mysql", "mariadb", "postgres", "rustfs":
		return true
	}
	return false
}

// EnsureSiteState creates the state a stateful service holds for one site: a
// database for the db families, a bucket for object storage. Both creators look
// the entity up before creating it, so this is idempotent and safe to call on
// every install, reinstall and link. Returns a short description of what was
// created, empty when the state was already there or the family holds none.
func EnsureSiteState(serviceName string, s config.Site) (string, error) {
	switch ServiceFamily(serviceName) {
	case "mysql", "mariadb", "postgres":
		dbName := resolveDBName(s)
		if dbName == "" {
			return "", fmt.Errorf("could not resolve db name")
		}
		created, err := reprovDB(serviceName, dbName)
		if err != nil {
			return "", fmt.Errorf("create db %s: %w", dbName, err)
		}
		if !created {
			return "", nil
		}
		return "created db " + dbName, nil

	case "rustfs":
		bucket := resolveBucketName(s)
		if bucket == "" {
			return "", fmt.Errorf("could not resolve bucket name")
		}
		created, err := reprovBucket(bucket)
		if err != nil {
			return "", fmt.Errorf("create bucket %s: %w", bucket, err)
		}
		if !created {
			return "", nil
		}
		return "created bucket " + bucket, nil
	}
	return "", nil
}

// ReprovisionLinkedSites recreates per-site state on a service that has just
// been installed or reinstalled, so every site already linked to it finds its
// database or bucket where it expects. Idempotent through EnsureSiteState:
// existing state is left alone and reported as nothing done.
//
// Per-site failures are collected and joined; the loop continues so one
// misconfigured site doesn't block the rest.
func ReprovisionLinkedSites(serviceName string, emit func(PhaseEvent)) error {
	if emit == nil {
		emit = func(PhaseEvent) {}
	}

	family := ServiceFamily(serviceName)
	if !familyHoldsSiteState(family) {
		emit(PhaseEvent{Phase: "reprovisioning_skipped", Message: fmt.Sprintf("family %q has no per-site state to recreate", family)})
		return nil
	}

	sites := config.SitesUsingService(serviceName)
	if len(sites) == 0 {
		return nil
	}

	emit(PhaseEvent{Phase: "reprovisioning_sites", Message: fmt.Sprintf("%d site(s)", len(sites))})

	var errs []error
	for _, s := range sites {
		detail, err := EnsureSiteState(serviceName, s)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name, err))
			continue
		}
		if detail != "" {
			emit(PhaseEvent{Phase: "reprovisioning_site", Message: fmt.Sprintf("%s: %s", s.Name, detail)})
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func resolveDBName(s config.Site) string {
	if proj, err := config.LoadProjectConfig(s.Path); err == nil && proj != nil {
		if proj.DB.Database != "" {
			return proj.DB.Database
		}
	}
	if v := envfile.ReadKey(filepath.Join(s.Path, ".env"), "DB_DATABASE"); v != "" {
		return v
	}
	return config.SiteSlug(s.Name)
}

func resolveBucketName(s config.Site) string {
	// .env may declare AWS_BUCKET=<name> to point at a non-default bucket.
	// An empty literal (`AWS_BUCKET=`) means the user hasn't customized;
	// fall through to the site name rather than feeding "" to S3BucketName
	// (which would normalize it to the placeholder string "lerd" and
	// collide every site that left AWS_BUCKET= empty onto the same bucket).
	if v := envfile.ReadKey(filepath.Join(s.Path, ".env"), "AWS_BUCKET"); v != "" {
		return S3BucketName(v)
	}
	return S3BucketName(s.Name)
}
