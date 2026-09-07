package reqstats

import "time"

// excludeSchema is the table of routes the user has asked lerd to stop watching.
// Exclusions are keyed by the bare site name, never a "<site>/<branch>" worktree
// key, so a route silenced once stays silenced on every branch of that project.
const excludeSchema = `
CREATE TABLE IF NOT EXISTS route_excludes (
  site  TEXT    NOT NULL,
  route TEXT    NOT NULL,
  at_ms INTEGER NOT NULL,
  PRIMARY KEY (site, route)
);`

// ExcludeRoute silences a route for a site: no new request on it is recorded and
// nothing already stored for it is shown. Takes either a site name or a worktree
// key, since the caller holds whichever the view was rendered for. Repeating an
// exclusion is a no-op.
func (s *Store) ExcludeRoute(siteKey, route string) error {
	site, _ := SplitKey(siteKey)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO route_excludes(site, route, at_ms) VALUES(?,?,?)`,
		site, route, time.Now().UnixMilli())
	return err
}

// UnexcludeRoute puts a route back under observation. Requests recorded while it
// was excluded were never stored, so the route reappears only as new traffic
// arrives.
func (s *Store) UnexcludeRoute(siteKey, route string) error {
	site, _ := SplitKey(siteKey)
	_, err := s.db.Exec(`DELETE FROM route_excludes WHERE site = ? AND route = ?`, site, route)
	return err
}

// ExcludedRoutes lists a site's excluded routes, oldest exclusion first, so the
// dashboard can render them in the order they were silenced.
func (s *Store) ExcludedRoutes(siteKey string) ([]string, error) {
	site, _ := SplitKey(siteKey)
	rows, err := s.db.Query(`SELECT route FROM route_excludes WHERE site = ? ORDER BY at_ms ASC`, site)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var route string
		if err := rows.Scan(&route); err != nil {
			return nil, err
		}
		out = append(out, route)
	}
	return out, rows.Err()
}

// AllExcludedRoutes returns every exclusion keyed by site, so the watcher can
// hold the whole set in memory and answer the ingest path without a query per
// request.
func (s *Store) AllExcludedRoutes() (map[string]map[string]bool, error) {
	rows, err := s.db.Query(`SELECT site, route FROM route_excludes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]bool{}
	for rows.Next() {
		var site, route string
		if err := rows.Scan(&site, &route); err != nil {
			return nil, err
		}
		if out[site] == nil {
			out[site] = map[string]bool{}
		}
		out[site][route] = true
	}
	return out, rows.Err()
}

// excludedSet reads one site's exclusions as a set, for the read paths that have
// to drop stored rows the user has since silenced. A failed read yields an empty
// set, so the view still renders rather than erroring on a filter.
func (s *Store) excludedSet(siteKey string) map[string]bool {
	routes, err := s.ExcludedRoutes(siteKey)
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(routes))
	for _, r := range routes {
		set[r] = true
	}
	return set
}

// DeleteRoute drops every stored request on a route for a site, covering its
// worktree keys the way DeleteSite does, and returns how many rows went.
func (s *Store) DeleteRoute(siteKey, route string) (int64, error) {
	site, _ := SplitKey(siteKey)
	res, err := s.db.Exec(
		`DELETE FROM requests WHERE route = ? AND (site = ? OR instr(site, ?) = 1)`,
		route, site, site+"/")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteRequest drops a single recorded request, identified the way the recent
// list renders it: the site key it was served under, its timestamp, and its path.
func (s *Store) DeleteRequest(siteKey string, atMillis int64, uri string) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM requests WHERE site = ? AND at_ms = ? AND uri = ?`,
		siteKey, atMillis, uri)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
