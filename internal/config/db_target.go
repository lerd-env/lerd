package config

import (
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/envfile"
)

// EnvFileFor resolves the file a project keeps its configuration in, and the
// format it is written in, both declared by the framework definition. Laravel's
// dotenv .env is only the default: WordPress keeps PHP constants in
// wp-config.php, Magento a PHP array in app/etc/env.php, Symfony its real values
// in .env.local. Anything that wants to read a project's configuration goes
// through this rather than joining ".env" onto the site path.
func EnvFileFor(projectDir string) (file, format string) {
	fw := frameworkFor(projectDir)
	if fw == nil {
		return ".env", "dotenv"
	}
	return fw.Env.Resolve(projectDir)
}

// DBEnvBinding describes how a project's env file addresses its database: the
// file and format holding it, and the keys naming the host container and the
// database itself. It is what a writer aims at; readers want DBTargetFor.
type DBEnvBinding struct {
	File    string
	Format  string
	HostKey string
	NameKey string
	// URLKey holds the whole connection, for a framework with no separate keys:
	// Symfony's DATABASE_URL, or Yii's PDO dsn.
	URLKey string
}

// Database is the database name this binding reads from an env.
func (b DBEnvBinding) Database(read func(string) string) string {
	if b.URLKey != "" {
		return DBNameInURL(read(b.URLKey))
	}
	return read(b.NameKey)
}

// NameUpdate is the env change that points this binding at another database.
func (b DBEnvBinding) NameUpdate(read func(string) string, name string) map[string]string {
	if b.URLKey != "" {
		return map[string]string{b.URLKey: DBURLWithName(read(b.URLKey), name)}
	}
	return map[string]string{b.NameKey: name}
}

// DBTarget is a database a project points at: the lerd service holding it and
// the name of the database inside that service.
type DBTarget struct {
	Service  string
	Database string
}

// dbServiceBinding is one declared database service and how the framework
// addresses it.
type dbServiceBinding struct {
	DBEnvBinding
	service string
	detect  []FrameworkServiceDetect
	keys    []string
}

// defaultDBEnvBinding is the shape Laravel gave everyone, and what a project
// whose framework declares no database service falls back to.
func defaultDBEnvBinding() DBEnvBinding {
	return DBEnvBinding{File: ".env", Format: "dotenv", HostKey: "DB_HOST", NameKey: "DB_DATABASE"}
}

// frameworkForCache keeps frameworkFor answers for frameworkForTTL: the services
// snapshot asks per site per service, and each miss reads every definition from
// disk. A changed definition shows once its entry expires.
var frameworkForCache struct {
	sync.Mutex
	entries map[string]frameworkForEntry
}

type frameworkForEntry struct {
	fw *Framework
	at time.Time
}

const frameworkForTTL = 3 * time.Second

// frameworkFor returns the framework definition governing a project directory,
// or nil when none is detected. Callers must not modify the result.
func frameworkFor(projectDir string) *Framework {
	frameworkForCache.Lock()
	e, ok := frameworkForCache.entries[projectDir]
	frameworkForCache.Unlock()
	if ok && time.Since(e.at) < frameworkForTTL {
		return e.fw
	}
	// Detected unlocked: a definition download must not stall other sites' lookups.
	fw := detectFrameworkFor(projectDir)
	frameworkForCache.Lock()
	defer frameworkForCache.Unlock()
	if frameworkForCache.entries == nil {
		frameworkForCache.entries = map[string]frameworkForEntry{}
	}
	for dir, old := range frameworkForCache.entries {
		if time.Since(old.at) >= frameworkForTTL {
			delete(frameworkForCache.entries, dir)
		}
	}
	frameworkForCache.entries[projectDir] = frameworkForEntry{fw: fw, at: time.Now()}
	return fw
}

func detectFrameworkFor(projectDir string) *Framework {
	name, ok := DetectFrameworkForDir(projectDir)
	if !ok {
		return nil
	}
	fw, ok := GetFrameworkForDir(name, projectDir)
	if !ok {
		return nil
	}
	return fw
}

// dbEnvBindings derives from the framework declaration how each database
// service it supports is addressed, ordered so the primary one is picked the
// same way every run, along with the env file and format they all read from.
// The keys come out of the service vars: the one set to {{site}} names the
// database, the one set to a lerd-<service> container names the host, and a var
// carrying both inside a URL is the DSN.
func dbEnvBindings(projectDir string) (bindings []dbServiceBinding, file, format string) {
	file, format = ".env", "dotenv"
	fw := frameworkFor(projectDir)
	if fw == nil {
		return nil, file, format
	}
	file, format = fw.Env.Resolve(projectDir)
	if len(fw.Env.Services) == 0 {
		return nil, file, format
	}
	var out []dbServiceBinding
	for name, def := range fw.Env.Services {
		if name == "sqlite" || !IsDBServiceName(name) {
			continue
		}
		b := dbServiceBinding{
			DBEnvBinding: DBEnvBinding{File: file, Format: format},
			service:      name,
			detect:       def.Detect,
		}
		for _, kv := range def.Vars {
			key, val, found := strings.Cut(kv, "=")
			if !found {
				continue
			}
			b.keys = append(b.keys, key)
			switch {
			case val == "{{site}}":
				b.NameKey = key
			case strings.HasPrefix(val, "lerd-"):
				b.HostKey = key
			case strings.Contains(val, "{{site}}") && strings.Contains(val, "lerd-"):
				b.URLKey = key
			}
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := dbServiceRank(out[i].service), dbServiceRank(out[j].service)
		if ri != rj {
			return ri < rj
		}
		return out[i].service < out[j].service
	})
	return out, file, format
}

// dbServiceRank orders the database services a framework may declare, so a
// definition supporting several of them resolves to the same primary every run.
func dbServiceRank(name string) int {
	families := []string{"mysql", "mariadb", "postgres", "mongo"}
	for i, family := range families {
		if FamilyOfName(name) == family {
			return i
		}
	}
	return len(families)
}

// DBEnvBindingFor returns how a project addresses its database: the declared
// service its env matches, then the first declared, then the default shape. A
// framework encoding its database in a DSN has only the URLKey.
func DBEnvBindingFor(projectDir string) DBEnvBinding {
	bindings, file, format := dbEnvBindings(projectDir)
	if len(bindings) == 0 {
		return defaultDBEnvBinding()
	}
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	var first *DBEnvBinding
	for _, b := range bindings {
		if b.URLKey == "" && (b.NameKey == "" || b.HostKey == "") {
			continue
		}
		if serviceDetectedIn(b.detect, vals) {
			return b.DBEnvBinding
		}
		if first == nil {
			binding := b.DBEnvBinding
			first = &binding
		}
	}
	if first != nil {
		return *first
	}
	return defaultDBEnvBinding()
}

// DBTargetFor returns the database a project uses as its framework declares it:
// read from the file the definition names, in the format it names, at the keys
// it names. ok is false when no framework declares a database service the
// project's env file matches, leaving the caller free to fall back to its own
// inference.
func DBTargetFor(projectDir string) (DBTarget, bool) {
	bindings, file, format := dbEnvBindings(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	for _, t := range declaredDBTargets(projectDir, bindings, vals) {
		return t, true
	}
	return DBTarget{}, false
}

// DBTargetsFor returns every lerd-managed database a project points at: the
// ones its framework declares, plus the default keys and any connection string
// aimed at a lerd engine, so a project whose framework lerd doesn't recognise
// still shows as the owner of its database.
func DBTargetsFor(projectDir string) []DBTarget {
	if svc := ExternalDBServiceFor(projectDir); svc != nil {
		if name := DBEndpointFor(projectDir).Database; name != "" {
			return []DBTarget{{Service: svc.Name, Database: name}}
		}
	}
	bindings, file, format := dbEnvBindings(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)

	var out []DBTarget
	claimed := map[string]bool{}
	add := func(t DBTarget) {
		if t.Service == "" || t.Database == "" || claimed[t.Service] {
			return
		}
		claimed[t.Service] = true
		out = append(out, t)
	}
	for _, t := range declaredDBTargets(projectDir, bindings, vals) {
		add(t)
	}
	// The default pair is Laravel's vocabulary and only ever names a database an
	// engine serves. Writing env vars sets keys and never removes them, so a
	// project moved to SQLite keeps its DB_HOST, and reading the pair regardless
	// would hand back a file path as a schema some lerd engine is missing.
	if serverDBDriver(vals["DB_CONNECTION"]) {
		add(DBTarget{
			Service:  dbServiceFor(projectDir, vals["DB_HOST"], ""),
			Database: vals["DB_DATABASE"],
		})
	}
	for _, t := range dsnTargets(vals) {
		add(t)
	}
	return out
}

// IsServerDBDriver reports whether a driver value names a database an engine
// serves rather than a file, for callers outside this package.
func IsServerDBDriver(v string) bool { return serverDBDriver(v) }

// serverDBDriver reports whether a driver value names a database served by an
// engine, the only kind a lerd service can hold. An unset value counts: a
// project that names no driver is read by its host and database keys as before.
func serverDBDriver(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "mysql", "mariadb", "pgsql", "postgres", "postgresql":
		return true
	}
	return false
}

// declaredDBTargets resolves the databases the framework declaration accounts
// for, in the order the declaration ranks them.
func declaredDBTargets(projectDir string, bindings []dbServiceBinding, vals map[string]string) []DBTarget {
	var out []DBTarget
	for _, b := range bindings {
		if !serviceDetectedIn(b.detect, vals) {
			continue
		}
		t := DBTarget{}
		switch {
		case b.URLKey != "":
			t = dsnTarget(vals[b.URLKey])
		case b.NameKey != "" && b.HostKey != "":
			t = DBTarget{
				Service:  dbServiceFor(projectDir, vals[b.HostKey], b.service),
				Database: vals[b.NameKey],
			}
		default:
			// A definition that detects a service without publishing its vars
			// still names a real database somewhere in the file it points at.
			t = declaredFallbackTarget(projectDir, b.service, vals)
		}
		if t.Service != "" && t.Database != "" {
			out = append(out, t)
		}
	}
	return out
}

// declaredFallbackTarget resolves a detected service whose declaration carries
// no vars, from the default keys and then from any connection string in the
// file aimed at that service's family.
func declaredFallbackTarget(projectDir, service string, vals map[string]string) DBTarget {
	if db := vals["DB_DATABASE"]; db != "" {
		return DBTarget{Service: dbServiceFor(projectDir, vals["DB_HOST"], service), Database: db}
	}
	family := FamilyOfName(service)
	for _, t := range dsnTargets(vals) {
		if FamilyOfName(t.Service) == family {
			return t
		}
	}
	return DBTarget{}
}

// serviceDetectedIn reports whether any of a declared service's detect rules
// matches the project's env values. A declaration carrying no rules has nothing
// to rule it out, so it is taken as applying and the values read at its keys
// decide whether it resolves to a database at all.
func serviceDetectedIn(detect []FrameworkServiceDetect, vals map[string]string) bool {
	return DetectRulesMatch(detect, vals)
}

// DBServiceFor returns the lerd service backing a project's database, given the
// host value read from its env file. Empty when the database is not one lerd
// manages.
func DBServiceFor(projectDir, host string) string {
	return dbServiceFor(projectDir, host, "")
}

// dbServiceFor returns the lerd service backing a database, from the host value
// the project's env file holds. Container and PHP sites name it directly
// (lerd-postgres-18 -> postgres-18); a host-proxy site rewrites the host to
// loopback, so the service is recovered from the database entry in its
// .lerd.yaml. An empty host falls back to the service the framework declared
// the block for, and a host lerd doesn't manage resolves to nothing, because
// that database is not ours.
func dbServiceFor(projectDir, host, declared string) string {
	host = strings.TrimSpace(host)
	if svc, ok := strings.CutPrefix(host, "lerd-"); ok && svc != "" {
		return strings.TrimSuffix(strings.Split(svc, ":")[0], "/")
	}
	// A host lerd does not serve is somebody else's database, whatever the
	// project's .lerd.yaml happens to list. Only the host-proxy rewrite, which
	// lands on loopback, is recovered from that list.
	if host != "" && !isLoopbackHost(host) {
		return ""
	}
	if proj, err := LoadProjectConfig(projectDir); err == nil {
		for _, svc := range proj.Services {
			if IsDBServiceName(svc.Name) && svc.Name != "sqlite" {
				return svc.Name
			}
		}
	}
	if host == "" {
		return declared
	}
	return ""
}

// WorktreeDBService is the lerd service holding a project's database, or "" when
// no lerd service does, read from the project's env.
func WorktreeDBService(projectDir string) string {
	if svc := ExternalDBServiceFor(projectDir); svc != nil {
		return svc.Name
	}
	binding := DBEnvBindingFor(projectDir)
	read := envfile.Reader(filepath.Join(projectDir, binding.File), binding.Format)
	host := read(binding.HostKey)
	if binding.URLKey != "" {
		host = endpointFromURL(read(binding.URLKey)).Host
	}
	service := DBServiceFor(projectDir, host)
	if service == "" {
		return ""
	}
	if _, external := ExternalDBHost(projectDir, service); external {
		return ""
	}
	return service
}

// DBEndpoint is where a project's env says its database lives, whichever way the
// framework spells it: separate keys, a host:port, or one connection string.
type DBEndpoint struct {
	Host     string
	Port     string
	Database string
}

// DBEndpointFor reads a project's database endpoint from its env.
func DBEndpointFor(projectDir string) DBEndpoint {
	bindings, file, format := dbEnvBindings(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	for _, b := range bindings {
		if b.URLKey != "" && serviceDetectedIn(b.detect, vals) {
			return endpointFromURL(vals[b.URLKey])
		}
	}
	binding := DBEnvBindingFor(projectDir)
	read := envfile.Reader(filepath.Join(projectDir, binding.File), binding.Format)
	e := DBEndpoint{Host: strings.TrimSpace(read(binding.HostKey)), Port: read("DB_PORT"), Database: read(binding.NameKey)}
	if host, port, err := net.SplitHostPort(e.Host); err == nil {
		e.Host, e.Port = host, port
	}
	return e
}

// DBURLFor is the key a framework keeps its whole connection in and the value a
// project has there, read from the file the framework declares. The key is "" for
// a framework that spells the connection out in separate keys.
func DBURLFor(projectDir string) (key, value string) {
	bindings, file, format := dbEnvBindings(projectDir)
	for _, b := range bindings {
		if b.URLKey != "" {
			return b.URLKey, envfile.Values(filepath.Join(projectDir, file), format)[b.URLKey]
		}
	}
	return "", ""
}

// DBURLConnection is the values a URL-keyed framework declares for its database
// that a project holds literally: the URL, and for Yii the login beside it.
func DBURLConnection(projectDir string) map[string]string {
	bindings, file, format := dbEnvBindings(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	for _, b := range bindings {
		if b.URLKey == "" || !serviceDetectedIn(b.detect, vals) {
			continue
		}
		out := map[string]string{}
		for _, k := range b.keys {
			if v, ok := vals[k]; ok {
				out[k] = v
			}
		}
		return out
	}
	return nil
}

// SameDBURLHost reports whether two connection values reach the same server, a
// port left out meaning the driver's default.
func SameDBURLHost(a, b string) bool {
	x, y := endpointFromURL(a), endpointFromURL(b)
	return x.Host == y.Host && effectivePort(a, x) == effectivePort(b, y)
}

func effectivePort(raw string, e DBEndpoint) string {
	if e.Port != "" {
		return e.Port
	}
	raw = strings.ToLower(strings.TrimLeft(raw, `"'`))
	switch {
	case strings.HasPrefix(raw, "mysql"), strings.HasPrefix(raw, "maria"):
		return "3306"
	case strings.HasPrefix(raw, "pgsql"), strings.HasPrefix(raw, "postgres"):
		return "5432"
	}
	return ""
}

// DBNameInURL is the database a connection URL names.
func DBNameInURL(raw string) string {
	return endpointFromURL(raw).Database
}

var pdoDBName = regexp.MustCompile(`dbname=[^;"']*`)

// DBURLWithName swaps the database in a connection URL by hand, since a rebuilt
// URL would re-encode a password the framework reads as written.
func DBURLWithName(raw, name string) string {
	_, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return pdoDBName.ReplaceAllLiteralString(raw, "dbname="+name)
	}
	schemeEnd := len(raw) - len(rest)
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return raw
	}
	start := schemeEnd + slash + 1
	end := len(raw)
	if i := strings.IndexAny(raw[start:], `?#"'`); i >= 0 {
		end = start + i
	}
	return raw[:start] + url.PathEscape(name) + raw[end:]
}

func endpointFromURL(raw string) DBEndpoint {
	raw = strings.Trim(raw, `"'`)
	if !strings.Contains(raw, "://") {
		return endpointFromPDO(raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return DBEndpoint{}
	}
	return DBEndpoint{Host: u.Hostname(), Port: u.Port(), Database: strings.TrimPrefix(u.Path, "/")}
}

// endpointFromPDO reads a PDO DSN, mysql:host=h;port=3306;dbname=shop.
func endpointFromPDO(raw string) DBEndpoint {
	var e DBEndpoint
	_, params, ok := strings.Cut(raw, ":")
	if !ok {
		return e
	}
	for _, part := range strings.Split(params, ";") {
		key, value, _ := strings.Cut(part, "=")
		switch strings.TrimSpace(key) {
		case "host":
			e.Host = value
		case "port":
			e.Port = value
		case "dbname":
			e.Database = value
		}
	}
	return e
}

// ExternalDBHost returns the host a project's env names when its database is not
// on the lerd service given. Loopback only reaches lerd through a port lerd
// publishes for that service; on any other it is a server on the machine.
func ExternalDBHost(projectDir, service string) (string, bool) {
	endpoint := DBEndpointFor(projectDir)
	host := endpoint.Host
	if host == "" {
		return "", false
	}
	if _, marked := envfile.ReadOverride(projectDir); marked[service] {
		return host, true
	}
	if strings.HasPrefix(host, "lerd-") {
		return "", false
	}
	if !isLoopbackHost(host) {
		return host, true
	}
	port := endpoint.Port
	published := ServiceConfigFor(service).HostPorts()
	if port == "" || len(published) == 0 || slices.Contains(published, atoiOr(port)) {
		return "", false
	}
	return net.JoinHostPort(host, port), true
}

func atoiOr(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// isLoopbackHost reports whether a host value from an env file points back at
// this machine, in any of the spellings a project writes: with or without a
// port, and IPv6 in brackets.
func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if hostOnly, _, err := net.SplitHostPort(h); err == nil {
		h = hostOnly
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// dsnTargets returns the databases the values point at through a connection
// string, one per service. Keys are visited in sorted order: a project with
// more than one DSN against the same engine would otherwise be attributed to a
// different database on every read, since Go randomises map iteration.
func dsnTargets(vals map[string]string) []DBTarget {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []DBTarget
	seen := map[string]bool{}
	for _, k := range keys {
		t := dsnTarget(vals[k])
		if t.Service == "" || t.Database == "" || seen[t.Service] {
			continue
		}
		seen[t.Service] = true
		out = append(out, t)
	}
	return out
}

// dsnTarget parses a connection string aimed at a lerd database engine
// (mongodb://root:lerd@lerd-mongo:27017/jobs) into its service and database.
// A value that is not such a URL yields the zero target.
func dsnTarget(value string) DBTarget {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "lerd-") {
		return DBTarget{}
	}
	e := endpointFromURL(value)
	service, ok := strings.CutPrefix(e.Host, "lerd-")
	if !ok || service == "" || !dsnServiceHoldsDatabases(service) {
		return DBTarget{}
	}
	if e.Database == "" || strings.Contains(e.Database, "/") {
		return DBTarget{}
	}
	return DBTarget{Service: service, Database: e.Database}
}

// dsnServiceHoldsDatabases keeps a cache or queue connection string out of the
// database walk: redis://lerd-redis:6379/0 ends in a database index, not a
// name. An engine whose preset isn't installed has no family to judge it by, so
// it stays in and the caller matches it against the engines it actually has.
func dsnServiceHoldsDatabases(service string) bool {
	if service == "sqlite" {
		return false
	}
	if FamilyOfName(service) == "" {
		return true
	}
	return IsDBServiceName(service)
}
