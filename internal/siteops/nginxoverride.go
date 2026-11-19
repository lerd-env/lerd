package siteops

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/cfgedit"
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/nginx"
	"github.com/geodro/lerd/internal/webserver"
)

// NginxTestFn / NginxReloadFn are indirections so tests can stub the
// podman-bound `nginx -t` and reload. The global-nginx editor reuses these so
// site and global saves share one validator/reload pair. Defaults are real.
var (
	NginxTestFn   = webserver.Test
	NginxReloadFn = webserver.Reload
)

// nginxSiteTemplate seeds the editor when no override exists yet. Everything
// is commented so the file is an inert no-op until the user opts in.
const nginxSiteTemplate = `# Lerd per-site nginx overrides.
#
# Included at the end of this site's server { } block. Lerd never overwrites
# this file, so edits survive vhost regeneration and ` + "`lerd update`" + `. Add
# directives valid inside a server block, then save to reload nginx.
#
# fastcgi_param and proxy_set_header do not belong here: nginx ignores the
# server-level set as soon as a location declares one of its own. Use the
# location-scope override for those.

# client_max_body_size 100m;
# location /ws { proxy_pass http://127.0.0.1:6001; proxy_http_version 1.1; }
`

// nginxLocationTemplate seeds the location-scope editor. Same never-overwritten
// contract; the examples are the directives that only work at this level.
const nginxLocationTemplate = `# Lerd per-site nginx overrides, location scope.
#
# Included at the end of the block that actually serves this site: the PHP
# location on an FPM site, location / on a proxied one. Directives nginx
# resolves per location land here, above all fastcgi_param and
# proxy_set_header, which a server-scope override cannot reach.

# Keep SERVER_NAME as the requested host instead of X-Forwarded-Host:
# fastcgi_param SERVER_NAME $host;
# fastcgi_param HTTP_HOST $host;
`

// NginxScope names which of a site's two override files to act on. Both are
// per site and never overwritten; they differ only in where the generated
// vhost includes them.
type NginxScope string

const (
	NginxServerScope   NginxScope = "server"
	NginxLocationScope NginxScope = "location"
)

// ParseNginxScope maps the wire value onto a scope, treating the empty string
// as the server scope so existing callers and stored URLs keep working.
func ParseNginxScope(s string) (NginxScope, error) {
	switch NginxScope(s) {
	case "", NginxServerScope:
		return NginxServerScope, nil
	case NginxLocationScope:
		return NginxLocationScope, nil
	}
	return "", fmt.Errorf("unknown nginx override scope %q", s)
}

// suffix is the custom.d filename tail for the scope. The location file must
// not match the server include's {domain}.conf* glob, hence the infix.
func (s NginxScope) suffix() string {
	if s == NginxLocationScope {
		return ".location.conf"
	}
	return ".conf"
}

func (s NginxScope) template() string {
	if s == NginxLocationScope {
		return nginxLocationTemplate
	}
	return nginxSiteTemplate
}

// CustomNginxPath is the on-disk path of a domain's custom override.
func CustomNginxPath(domain string, scope NginxScope) string {
	return filepath.Join(config.NginxCustomD(), domain+scope.suffix())
}

// nginxFile builds the cfgedit.File for a domain's custom override. Backups and
// write-staging live in custom.d.bkp/, kept off the custom.d/*.conf* glob.
func nginxFile(domain string, scope NginxScope) cfgedit.File {
	return cfgedit.File{
		Path:     CustomNginxPath(domain, scope),
		BkpDir:   config.NginxCustomDBkp(),
		BkpName:  domain + scope.suffix(),
		Template: scope.template(),
	}
}

// nginxScopes is every scope, for the whole-site operations (rename, inherit,
// remove) that have to carry both files.
var nginxScopes = []NginxScope{NginxServerScope, NginxLocationScope}

// nginxValidate runs `nginx -t` (via the stubbable indirection) so cfgedit can
// pre-flight a save without importing nginx itself.
func nginxValidate(string) (string, error) { return NginxTestFn() }

// ReadCustomNginx returns the saved override, or the seeded template
// (Exists=false) when nothing is on disk yet.
func ReadCustomNginx(domain string, scope NginxScope) (cfgedit.Content, error) {
	return nginxFile(domain, scope).Read()
}

// SaveCustomNginx writes, validates with `nginx -t`, rolls back on a failure
// that names our file, and reloads nginx.
func SaveCustomNginx(domain string, scope NginxScope, content string, backup bool) (cfgedit.SaveResult, error) {
	return nginxFile(domain, scope).Save(content, cfgedit.SaveOpts{
		Backup:   backup,
		Validate: nginxValidate,
		Owns:     cfgedit.MentionsFile,
		Apply:    func() error { return NginxReloadFn() },
	})
}

// ResetCustomNginx deletes the override and reloads nginx. Backups are kept.
func ResetCustomNginx(domain string, scope NginxScope) error {
	return nginxFile(domain, scope).Reset(func() error { return NginxReloadFn() })
}

// ListCustomNginxBackups returns a domain's override backups, newest first.
func ListCustomNginxBackups(domain string, scope NginxScope) ([]cfgedit.Backup, error) {
	return nginxFile(domain, scope).ListBackups()
}

// ReadCustomNginxBackup returns the raw bytes of one backup (os.ErrNotExist
// when the name is invalid or the file is gone).
func ReadCustomNginxBackup(domain string, scope NginxScope, name string) ([]byte, error) {
	return nginxFile(domain, scope).ReadBackup(name)
}

// RestoreCustomNginx swaps a backup over the live override and reloads nginx.
func RestoreCustomNginx(domain string, scope NginxScope, name string) (cfgedit.RestoreResult, error) {
	return nginxFile(domain, scope).Restore(name, func() error { return NginxReloadFn() })
}

// ValidNginxBackupName reports whether name is a well-formed backup for domain.
func ValidNginxBackupName(domain string, scope NginxScope, name string) bool {
	return nginxFile(domain, scope).ValidBackupName(name)
}

// InheritCustomNginxConfig seeds a new worktree's overrides from its parent's:
// it copies each scope's file only when the parent has one and the worktree
// does not. Callers must invoke it only on genuine worktree creation — running
// it on every resync would resurrect an override the user deliberately reset
// (it can't tell "new" from "reset").
func InheritCustomNginxConfig(parentDomain, worktreeDomain string) error {
	if parentDomain == worktreeDomain {
		return nil
	}
	for _, scope := range nginxScopes {
		if err := inheritOneNginxScope(parentDomain, worktreeDomain, scope); err != nil {
			return err
		}
	}
	return nil
}

func inheritOneNginxScope(parentDomain, worktreeDomain string, scope NginxScope) error {
	dst := CustomNginxPath(worktreeDomain, scope)
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := os.ReadFile(CustomNginxPath(parentDomain, scope))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(config.NginxCustomD(), 0o755); err != nil {
		return err
	}
	return nginx.WriteFileAtomic(dst, data, 0o644)
}

// RemoveCustomNginxConfig deletes a worktree's live overrides and every
// timestamped backup for that domain. Used when a worktree is removed.
func RemoveCustomNginxConfig(domain string) error {
	var firstErr error
	for _, scope := range nginxScopes {
		if err := os.Remove(CustomNginxPath(domain, scope)); err != nil && !os.IsNotExist(err) {
			return err
		}
		f := nginxFile(domain, scope)
		backups, err := f.ListBackups()
		if err != nil {
			return err
		}
		for _, b := range backups {
			if err := os.Remove(filepath.Join(f.BkpDir, b.Name)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
