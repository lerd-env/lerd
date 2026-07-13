package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	gitpkg "github.com/geodro/lerd/internal/git"
)

// DropOrphanedWorktreeDBs scans the registry for orphaned worktree state
// (LAN shares first, isolated databases last) and tears it down. Database
// drop is intentionally the LAST step so any earlier failure (LAN proxy
// stop, daemon notify, registry write) leaves the data intact and the user
// can recover by re-adding the worktree without losing migrations or seed
// data.
func DropOrphanedWorktreeDBs(site *config.Site) {
	live := liveWorktreeBranches(site)
	DropOrphanedWorktreeLANShares(site, live)
	dbEntries, _ := config.WorktreeDBsForSite(site.Name)
	for _, e := range dbEntries {
		if live[e.Branch] {
			continue
		}
		_, _ = DropDatabase(e.Service, e.DBName)
		_, _, _ = config.RemoveWorktreeDB(e.Site, e.Branch)
	}
}

func liveWorktreeBranches(site *config.Site) map[string]bool {
	out := map[string]bool{}
	worktrees, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return out
	}
	for _, w := range worktrees {
		out[w.Branch] = true
	}
	return out
}

// SetWorktreeDBIsolated is the shared lifecycle helper used by both the HTTP
// handler and the `lerd db:isolate` / `lerd db:share` CLI commands. On enable
// it creates `<parent_db>_<sanitized_branch>` in the same service the parent
// uses, optionally clones from `source` (empty / "main" / another isolated
// branch), records the worktree-DB pair in the registry, and rewrites the
// database-name key in the worktree's env file. The file, format and key are
// resolved from the framework definition (see resolveDBEnvBinding), so Laravel's
// DB_DATABASE and Magento's db.connection.default.dbname are handled alike. On
// disable it drops the DB and restores the parent's value. Idempotent.
// resolveDBService returns the lerd service backing the parent site's database.
// Container/PHP sites name it directly in DB_HOST (lerd-postgres -> postgres);
// host-proxy sites rewrite DB_HOST to loopback, so the service is recovered from
// the mysql/mariadb/postgres-family entry in the site's .lerd.yaml instead.
// Returns "" when no lerd-managed database can be resolved.
func resolveDBService(site *config.Site, parentHost string) string {
	if strings.HasPrefix(parentHost, "lerd-") {
		return strings.TrimPrefix(parentHost, "lerd-")
	}
	proj, err := config.LoadProjectConfig(site.Path)
	if err != nil {
		return ""
	}
	for _, svc := range proj.Services {
		switch config.FamilyOfName(svc.Name) {
		case "mysql", "mariadb", "postgres":
			return svc.Name
		}
	}
	return ""
}

// dbEnvBinding describes how a framework's env file addresses its primary
// database: the file and format to read and write, and the keys holding the DB
// host (a lerd-<service> container name) and the database name. It is resolved
// from the framework definition's mysql/mariadb/postgres service vars, so a
// Laravel site (DB_HOST / DB_DATABASE), a Magento site
// (db.connection.default.host / .dbname) and any future framework are addressed
// the same way. A framework that encodes its database in a single DSN (Symfony's
// DATABASE_URL) has no standalone name key, so it falls back to the Laravel-shaped
// default and db:isolate reports it as unmanaged, exactly as before.
type dbEnvBinding struct {
	file    string
	format  string
	hostKey string
	nameKey string
}

func resolveDBEnvBinding(sitePath string) dbEnvBinding {
	def := dbEnvBinding{file: ".env", format: "dotenv", hostKey: "DB_HOST", nameKey: "DB_DATABASE"}
	fwName, ok := config.DetectFrameworkForDir(sitePath)
	if !ok {
		return def
	}
	fw, ok := config.GetFrameworkForDir(fwName, sitePath)
	if !ok {
		return def
	}
	file, format := fw.Env.Resolve(sitePath)
	for _, family := range []string{"mysql", "mariadb", "postgres"} {
		svc, ok := fw.Env.Services[family]
		if !ok {
			continue
		}
		b := dbEnvBinding{file: file, format: format}
		for _, v := range svc.Vars {
			key, val, found := strings.Cut(v, "=")
			if !found {
				continue
			}
			switch {
			case val == "{{site}}":
				b.nameKey = key
			case strings.HasPrefix(val, "lerd-"):
				b.hostKey = key
			}
		}
		if b.nameKey != "" && b.hostKey != "" {
			return b
		}
	}
	return def
}

// applyDBEnvUpdate writes updates to a worktree env file through the writer that
// matches its format, so php-array (Magento) and php-const (WordPress) files are
// rewritten correctly rather than corrupted by the dotenv writer.
func applyDBEnvUpdate(path, format string, updates map[string]string) error {
	switch format {
	case "php-const":
		return envfile.ApplyPhpConstUpdates(path, updates)
	case "php-array":
		return envfile.ApplyPhpArrayUpdates(path, updates)
	default:
		return envfile.ApplyUpdates(path, updates)
	}
}

func SetWorktreeDBIsolated(site *config.Site, branch string, isolated bool, source string) error {
	worktrees, err := gitpkg.DetectWorktrees(site.Path, site.PrimaryDomain())
	if err != nil {
		return fmt.Errorf("detecting worktrees: %w", err)
	}
	var wt gitpkg.Worktree
	found := false
	for _, w := range worktrees {
		if w.Branch == branch {
			wt = w
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown worktree branch %q", branch)
	}

	binding := resolveDBEnvBinding(site.Path)
	readParent := envfile.Reader(filepath.Join(site.Path, binding.file), binding.format)
	parentDB := readParent(binding.nameKey)
	parentHost := readParent(binding.hostKey)
	service := resolveDBService(site, parentHost)
	if parentDB == "" || service == "" {
		return fmt.Errorf("parent site does not use a lerd-managed mysql/postgres (%s=%q, %s=%q)", binding.hostKey, parentHost, binding.nameKey, parentDB)
	}
	dbName := WorktreeDBName(parentDB, branch)
	wtEnv := filepath.Join(wt.Path, binding.file)

	if isolated {
		if _, err := CreateDatabase(service, dbName); err != nil {
			return fmt.Errorf("creating database %q in %s: %w", dbName, service, err)
		}
		if cloneSrc := resolveCloneSource(site, branch, source, parentDB); cloneSrc != "" {
			if err := CloneDatabase(service, cloneSrc, dbName); err != nil {
				_, _ = DropDatabase(service, dbName)
				return err
			}
		}
		if err := config.AddWorktreeDB(config.WorktreeDBEntry{
			Site:    site.Name,
			Branch:  branch,
			Service: service,
			DBName:  dbName,
		}); err != nil {
			return fmt.Errorf("recording worktree db: %w", err)
		}
		if err := config.SetWorktreeDBIsolated(wt.Path, true); err != nil {
			return fmt.Errorf("updating .lerd.yaml: %w", err)
		}
		if err := applyDBEnvUpdate(wtEnv, binding.format, map[string]string{binding.nameKey: dbName}); err != nil {
			return fmt.Errorf("rewriting worktree env: %w", err)
		}
		return nil
	}

	if entry, removed, err := config.RemoveWorktreeDB(site.Name, branch); err == nil && removed {
		_, _ = DropDatabase(entry.Service, entry.DBName)
	}
	if err := config.SetWorktreeDBIsolated(wt.Path, false); err != nil {
		return fmt.Errorf("updating .lerd.yaml: %w", err)
	}
	if _, err := os.Stat(wtEnv); err == nil {
		if err := applyDBEnvUpdate(wtEnv, binding.format, map[string]string{binding.nameKey: parentDB}); err != nil {
			return fmt.Errorf("restoring worktree env: %w", err)
		}
	}
	return nil
}

// EnsureWorktreeIsolatedDB provisions the isolated database for a worktree that
// committed `db_isolated: true` in its .lerd.yaml but has no database yet. This
// is the case when a site is linked with a pre-existing worktree (or one is
// materialised on a daemon that was offline): the interactive add-time prompt
// that normally creates the DB never ran, so the worktree's vhost and workers
// come up but its .env points at a database that was never created.
//
// It is a no-op when the worktree didn't opt in or its DB is already in the
// registry, so it is safe to call on every scan. The database is seeded empty
// (the worktree's own migrations populate it); cloning from the parent stays an
// explicit `lerd db:isolate` / `lerd worktree add` choice. Returns true only
// when it created the database this call.
func EnsureWorktreeIsolatedDB(site *config.Site, branch, worktreePath string) (bool, error) {
	if !config.WorktreeDBIsolated(worktreePath) {
		return false, nil
	}
	if _, found, _ := config.FindWorktreeDB(site.Name, branch); found {
		return false, nil
	}
	if err := SetWorktreeDBIsolated(site, branch, true, "empty"); err != nil {
		return false, err
	}
	return true, nil
}

// WorktreeDBName mirrors the projectDBName convention so a parent named
// "acme_app" with branch "feat-x" becomes "acme_app_feat_x".
func WorktreeDBName(parentDB, branch string) string {
	return parentDB + "_" + config.SiteSlug(branch)
}

func resolveCloneSource(site *config.Site, branch, source, parentDB string) string {
	switch source {
	case "", "empty":
		return ""
	case "main":
		return parentDB
	default:
		if entry, ok, err := config.FindWorktreeDB(site.Name, source); err == nil && ok && source != branch {
			return entry.DBName
		}
		return ""
	}
}

// FindParentSiteForWorktree looks up the registered site whose worktrees
// contain dir. Returns (site, branch, true) on a match, otherwise (_, _, false).
func FindParentSiteForWorktree(dir string) (*config.Site, string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", false
	}
	reg, err := config.LoadSites()
	if err != nil {
		return nil, "", false
	}
	for i := range reg.Sites {
		s := &reg.Sites[i]
		if s.Ignored {
			continue
		}
		worktrees, err := gitpkg.DetectWorktrees(s.Path, s.PrimaryDomain())
		if err != nil {
			continue
		}
		for _, wt := range worktrees {
			if wt.Path == abs {
				return s, wt.Branch, true
			}
		}
	}
	return nil, "", false
}
