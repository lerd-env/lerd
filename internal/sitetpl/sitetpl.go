// Package sitetpl expands the {{…}} placeholders a framework definition may use
// in env vars, setup steps, and commands, so the store can declare a per-site
// value (a base URL, a database name) without lerd knowing the framework.
package sitetpl

import (
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/grouping"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
)

// Ctx holds the values available to placeholders. An empty field leaves its
// placeholder untouched rather than substituting an empty string.
type Ctx struct {
	Site   string // database / handle name (underscored)
	Bucket string // S3-safe bucket name (lowercase, hyphens)
	Domain string // primary domain (e.g. myapp.test)
	Scheme string // "http" or "https"
}

// versionedServices are the presets whose {{<name>_version}} placeholder resolves
// to the running container's image version.
var versionedServices = []string{"mysql", "postgres", "redis", "meilisearch"}

// Apply replaces {{site}}, {{site_testing}}, {{bucket}}, {{domain}}, {{scheme}},
// and the {{<service>_version}} placeholders in s.
func Apply(s string, ctx Ctx) string {
	if ctx.Site != "" {
		s = strings.ReplaceAll(s, "{{site}}", ctx.Site)
		s = strings.ReplaceAll(s, "{{site_testing}}", ctx.Site+"_testing")
	}
	if ctx.Bucket != "" {
		s = strings.ReplaceAll(s, "{{bucket}}", ctx.Bucket)
	}
	if ctx.Domain != "" {
		s = strings.ReplaceAll(s, "{{domain}}", ctx.Domain)
	}
	if ctx.Scheme != "" {
		s = strings.ReplaceAll(s, "{{scheme}}", ctx.Scheme)
	}
	for _, svc := range versionedServices {
		placeholder := "{{" + svc + "_version}}"
		if strings.Contains(s, placeholder) {
			s = strings.ReplaceAll(s, placeholder, podman.ServiceVersion("lerd-"+svc))
		}
	}
	return s
}

// DBName returns the database handle for the project at path: the site's slug,
// or the group main's database when the site is a shared-DB group secondary.
func DBName(path string) string {
	name := filepath.Base(path)
	if reg, err := config.LoadSites(); err == nil {
		for _, s := range reg.Sites {
			if s.Path == path {
				if shared, ok := grouping.SharedDBNameFor(&s); ok {
					return shared
				}
				name = s.Name
				break
			}
		}
	}
	return config.SiteSlug(name)
}

// ForSite builds the context for a registered site.
func ForSite(site *config.Site) Ctx {
	if site == nil {
		return Ctx{}
	}
	db := DBName(site.Path)
	scheme := "http"
	if site.Secured {
		scheme = "https"
	}
	return Ctx{
		Site:   db,
		Bucket: serviceops.S3BucketName(db),
		Domain: site.PrimaryDomain(),
		Scheme: scheme,
	}
}

// ForPath builds the context for the project at path, falling back to a
// name-only context when the path is not a registered site.
func ForPath(path string) Ctx {
	if reg, err := config.LoadSites(); err == nil {
		for i := range reg.Sites {
			if reg.Sites[i].Path == path {
				return ForSite(&reg.Sites[i])
			}
		}
	}
	db := DBName(path)
	return Ctx{Site: db, Bucket: serviceops.S3BucketName(db)}
}

// ExpandCommands returns cmds with placeholders expanded in each shell string.
func ExpandCommands(cmds []config.FrameworkCommand, ctx Ctx) []config.FrameworkCommand {
	out := make([]config.FrameworkCommand, len(cmds))
	copy(out, cmds)
	for i := range out {
		out[i].Command = Apply(out[i].Command, ctx)
	}
	return out
}
