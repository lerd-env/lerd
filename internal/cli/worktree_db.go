package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	gitpkg "github.com/geodro/lerd/internal/git"
	"github.com/geodro/lerd/internal/plugins"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/serviceops"
	"github.com/geodro/lerd/internal/sitedoctor"
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
		// A database whose drop failed is still out there, so its record stays
		// for the next cleanup to retry.
		if dropWorktreeDB(e) != nil {
			continue
		}
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

// applyDBEnvUpdate writes updates to a worktree env file through the writer that
// matches its format, so php-array (Magento) and php-const (WordPress) files are
// rewritten correctly rather than corrupted by the dotenv writer.
func applyDBEnvUpdate(path, format string, updates map[string]string) error {
	return envfile.ApplyUpdatesIn(path, format, updates)
}

// SetWorktreeDBIsolated is the shared lifecycle helper used by both the HTTP
// handler and the `lerd db:isolate` / `lerd db:share` CLI commands. On enable
// it creates `<parent_db>_<sanitized_branch>` in the same service the parent
// uses, optionally clones from `source` (empty / "main" / another isolated
// branch), records the worktree-DB pair in the registry, and rewrites the
// database-name key in the worktree's env file. The file, format and key are
// resolved from the framework definition (config.DBEnvBindingFor), so Laravel's
// DB_DATABASE and Magento's db.connection.default.dbname are handled alike. On
// disable it drops the DB and restores the parent's value. Idempotent.
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

	binding := config.DBEnvBindingFor(site.Path)
	readParent := envfile.Reader(filepath.Join(site.Path, binding.File), binding.Format)
	parentDB := binding.Database(readParent)
	service := config.WorktreeDBService(site.Path)
	if parentDB == "" || service == "" {
		return fmt.Errorf("the database in %s is not on a lerd service, so this worktree cannot get its own", binding.File)
	}
	dbName := WorktreeDBName(parentDB, branch)
	wtEnv := filepath.Join(wt.Path, binding.File)
	readWorktree := worktreeEnvReader(binding, site.Path, wt.Path)

	if isolated {
		if err := worktreeOnParentServer(binding, site, wt.Path); err != nil {
			return err
		}
		p := plugins.ProviderOf(plugins.JobWorktreeDatabase)
		if kept, ok, _ := config.FindWorktreeDB(site.Name, branch); ok && (kept.Provider != "" || p != nil) {
			return reconnectKeptDB(p, site, wt.Path, kept)
		}
		if p != nil {
			return isolateWithPlugin(p, site, wt.Path, branch, source, service, parentDB, dbName)
		}
		if !serviceops.DeclaresDatabaseAction(service, "create") || !serviceops.DeclaresDatabaseAction(service, "drop") {
			return fmt.Errorf("%s declares no way to create and drop databases, so this worktree cannot get its own", service)
		}
		if _, err := createDatabaseFn(service, dbName); err != nil {
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
		if err := setWorktreeDBIsolatedFlag(site.Path, wt.Path, true); err != nil {
			return err
		}
		if err := applyDBEnvUpdate(wtEnv, binding.Format, binding.NameUpdate(readWorktree, dbName)); err != nil {
			return fmt.Errorf("rewriting worktree env: %w", err)
		}
		return restartWorktreeWorkersFn(site, wt.Path)
	}

	restore := binding.NameUpdate(readWorktree, parentDB)
	if entry, ok, err := config.FindWorktreeDB(site.Name, branch); err == nil && ok {
		if err := dropWorktreeDB(entry); err != nil {
			return fmt.Errorf("dropping database %q: %w", entry.DBName, err)
		}
		if _, _, err := config.RemoveWorktreeDB(site.Name, branch); err != nil {
			return fmt.Errorf("removing registry entry: %w", err)
		}
		if entry.Provider != "" && binding.URLKey == "" {
			restore[binding.HostKey] = readParent(binding.HostKey)
		}
	}
	if err := setWorktreeDBIsolatedFlag(site.Path, wt.Path, false); err != nil {
		return err
	}
	if _, err := os.Stat(wtEnv); err == nil {
		if err := applyDBEnvUpdate(wtEnv, binding.Format, restore); err != nil {
			return fmt.Errorf("restoring worktree env: %w", err)
		}
	}
	return restartWorktreeWorkersFn(site, wt.Path)
}

// providedDB is what a plugin in charge of worktree databases answers create
// with: the database it made and, when it serves it elsewhere, the host.
type providedDB struct {
	Host     string `json:"host"`
	Database string `json:"database"`
}

// isolateWithPlugin hands the worktree's database to the plugin in charge of
// it. A failure leaves the worktree sharing the parent's database rather than
// falling back to a full copy, the cost the plugin is there to avoid.
func isolateWithPlugin(p *plugins.Plugin, site *config.Site, wtPath, branch, source, service, parentDB, dbName string) error {
	if source == "" {
		source = "empty"
	}
	out, err := p.Provide(plugins.JobWorktreeDatabase, "create", map[string]string{
		"site": site.Name, "site_path": site.Path, "branch": branch, "worktree_path": wtPath,
		"service": service, "parent_database": parentDB, "database": dbName, "source": source,
	})
	if err != nil {
		return err
	}
	var got providedDB
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&got); err != nil {
		return fmt.Errorf("plugin %s answered create with %w", p.Name, err)
	}
	if got.Database == "" {
		return fmt.Errorf("plugin %s answered create without a database", p.Name)
	}
	entry := config.WorktreeDBEntry{Site: site.Name, Branch: branch, Service: service, DBName: got.Database, Host: got.Host, Provider: p.Name}
	if err := worktreeHostFits(site, entry); err != nil {
		dropErr := dropWorktreeDB(entry)
		if dropErr == nil {
			return err
		}
		// Left behind, it needs its record for db:share to find and drop it.
		entry.Leftover = true
		return errors.Join(err, dropErr, config.AddWorktreeDB(entry))
	}
	if err := config.AddWorktreeDB(entry); err != nil {
		_ = dropWorktreeDB(entry)
		return fmt.Errorf("recording worktree db: %w", err)
	}
	return connectWorktreeDB(site, wtPath, entry)
}

// reconnectKeptDB points the worktree back at the database kept for it. Only
// whoever made that database can serve it, so one made by a plugin that is no
// longer in charge is refused rather than replaced with a new, empty one.
func reconnectKeptDB(inCharge *plugins.Plugin, site *config.Site, wtPath string, kept config.WorktreeDBEntry) error {
	if kept.Provider != "" && (inCharge == nil || inCharge.Name != kept.Provider) {
		return fmt.Errorf("database %q was made by plugin %s, which is not in charge of worktree databases now; reinstall it to reach that database", kept.DBName, kept.Provider)
	}
	return connectWorktreeDB(site, wtPath, kept)
}

// connectWorktreeDB points the worktree at a kept or plugin-made database.
func connectWorktreeDB(site *config.Site, wtPath string, entry config.WorktreeDBEntry) error {
	if err := worktreeHostFits(site, entry); err != nil {
		return err
	}
	if err := worktreeOnParentServer(config.DBEnvBindingFor(site.Path), site, wtPath); err != nil {
		return err
	}
	if err := setWorktreeDBIsolatedFlag(site.Path, wtPath, true); err != nil {
		return err
	}
	binding := config.DBEnvBindingFor(site.Path)
	updates := binding.NameUpdate(worktreeEnvReader(binding, site.Path, wtPath), entry.DBName)
	if entry.Host != "" {
		updates[binding.HostKey] = entry.Host
	}
	if err := applyDBEnvUpdate(filepath.Join(wtPath, binding.File), binding.Format, updates); err != nil {
		return fmt.Errorf("rewriting worktree env: %w", err)
	}
	return restartWorktreeWorkersFn(site, wtPath)
}

// worktreeHostFits refuses a database served from another host on a framework
// that keeps its server in one connection value, which lerd only renames in.
func worktreeHostFits(site *config.Site, entry config.WorktreeDBEntry) error {
	if binding := config.DBEnvBindingFor(site.Path); entry.Host != "" && binding.URLKey != "" {
		return fmt.Errorf("%s keeps its server in %s, which cannot take the host a plugin serves %s from", site.Name, binding.URLKey, entry.DBName)
	}
	return nil
}

// worktreeOnParentServer refuses a worktree whose own connection names another
// server: its database is made on the parent's, where it could not reach it.
func worktreeOnParentServer(binding config.DBEnvBinding, site *config.Site, wtPath string) error {
	if binding.URLKey == "" {
		return nil
	}
	mine := envfile.Reader(filepath.Join(wtPath, binding.File), binding.Format)(binding.URLKey)
	parent := envfile.Reader(filepath.Join(site.Path, binding.File), binding.Format)(binding.URLKey)
	if mine == "" || config.SameDBURLHost(mine, parent) {
		return nil
	}
	return fmt.Errorf("this worktree's %s points at another server than %s, so it cannot reach a copy made there", binding.URLKey, site.Name)
}

// worktreeEnvReader reads the worktree's own value, so its connection options
// survive a rename, and the parent's when it has none yet.
func worktreeEnvReader(binding config.DBEnvBinding, sitePath, wtPath string) func(string) string {
	parent := envfile.Reader(filepath.Join(sitePath, binding.File), binding.Format)
	mine := envfile.Reader(filepath.Join(wtPath, binding.File), binding.Format)
	return func(key string) string {
		if v := mine(key); v != "" {
			return v
		}
		return parent(key)
	}
}

// dropWorktreeDB drops a worktree's isolated database through whoever made it.
func dropWorktreeDB(e config.WorktreeDBEntry) error {
	if e.Provider == "" {
		if !serviceops.DeclaresDatabaseAction(e.Service, "drop") {
			return fmt.Errorf("%s declares no way to drop a database, so %q is still there; drop it with DROP DATABASE in 'lerd db:shell'", e.Service, e.DBName)
		}
		_, err := DropDatabase(e.Service, e.DBName)
		return err
	}
	p, err := plugins.Find(e.Provider)
	if err != nil {
		return fmt.Errorf("database %q was made by plugin %s, which is no longer installed: %w", e.DBName, e.Provider, err)
	}
	_, err = p.Provide(plugins.JobWorktreeDatabase, "drop", map[string]string{
		"site": e.Site, "branch": e.Branch, "service": e.Service, "database": e.DBName,
	})
	return err
}

// createDatabaseFn creates a database in a lerd service; a seam for tests.
var createDatabaseFn = CreateDatabase

// restartWorktreeWorkersFn restarts a worktree's running workers; a seam for tests.
var restartWorktreeWorkersFn = restartRunningWorktreeWorkers

// restartRunningWorktreeWorkers restarts the workers running for the worktree
// at wtPath, so a process holding a database connection reconnects to the
// database its env now names.
func restartRunningWorktreeWorkers(site *config.Site, wtPath string) error {
	base := config.WorktreeUnitSlug(filepath.Base(wtPath))
	for _, w := range collectRunningWorktreeWorkersByBase(site, base) {
		unit := "lerd-" + w + "-" + site.Name + "-" + base
		if err := podman.RestartUnit(unit); err != nil {
			return fmt.Errorf("restarting %s: %w", unit, err)
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

// setWorktreeDBIsolatedFlag records the choice in the worktree's untracked
// override file, excluded from git first so it never shows up as a change.
func setWorktreeDBIsolatedFlag(sitePath, worktreePath string, isolated bool) error {
	if err := ensureLocalOverrideExcluded(sitePath); err != nil {
		return fmt.Errorf("excluding %s from git: %w", config.LocalOverrideFile, err)
	}
	if err := config.SetWorktreeDBIsolated(worktreePath, isolated); err != nil {
		return fmt.Errorf("updating %s: %w", config.LocalOverrideFile, err)
	}
	return nil
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
			if config.SamePath(wt.Path, abs) {
				return s, wt.Branch, true
			}
		}
	}
	return nil, "", false
}

// SeedWorktreeSQLite copies the parent's SQLite database into a new worktree.
// The file is gitignored, so the checkout arrives without it while its env still
// names it, and the site fails its first request. A copy rather than a shared
// file keeps the branch's migrations off main's data. The write-ahead log goes
// with it so recent writes still in it are not lost. Returns true when it copied.
func SeedWorktreeSQLite(site *config.Site, worktreePath string) (bool, error) {
	fw, _ := config.GetFrameworkForDir(site.Framework, site.Path)
	rel, ok := sitedoctor.ProjectSQLiteFile(site.Path, fw)
	if !ok {
		return false, nil
	}
	dst := filepath.Join(worktreePath, rel)
	if _, err := os.Lstat(dst); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	for _, suffix := range []string{"", "-wal"} {
		src := filepath.Join(site.Path, rel) + suffix
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := gitpkg.CopyTree(src, dst+suffix); err != nil {
			return false, fmt.Errorf("copying %s into the worktree: %w", rel+suffix, err)
		}
	}
	return true, nil
}

// WorktreeUsesSQLite reports whether the site's worktrees get their database as
// a copied file, which the mysql/postgres isolation choices cannot act on.
func WorktreeUsesSQLite(site *config.Site) bool {
	fw, _ := config.GetFrameworkForDir(site.Framework, site.Path)
	_, ok := sitedoctor.ProjectSQLiteFile(site.Path, fw)
	return ok
}
