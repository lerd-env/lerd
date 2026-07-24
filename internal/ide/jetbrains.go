// Package ide keeps a project's IDE-side pointers at lerd in sync. An IDE
// cannot discover a site's database on its own: the site's env file carries the
// container host and port, which are right inside the network and unusable from
// the machine, and no format is shared across IDEs for a project to declare one.
// JetBrains keeps its data sources in a plain XML file, so lerd maintains one
// entry there with host-facing coordinates.
package ide

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DataSource is one connection as the IDE stores it. Key identifies it inside a
// project, so an entry is updated in place across runs, and one whose database
// the project no longer uses can be told apart from one that is still wanted.
type DataSource struct {
	Key    string // stable within a project, e.g. the database name
	Name   string // shown in the IDE's database tool window
	Driver string // driver-ref, e.g. "postgresql"
	Class  string // JDBC driver class
	URL    string // full jdbc: URL
	User   string // the IDE authenticates with this, from its own sidecar
}

// ownedSuffix marks the entries lerd maintains, so one whose database the
// project has stopped using can be cleaned up without a record of what was
// written last time.
const ownedSuffix = " (lerd)"

// Owns reports whether a data source name is one lerd maintains.
func Owns(name string) bool { return strings.HasSuffix(name, ownedSuffix) }

const (
	dataSourcesFile = "dataSources.xml"
	localFile       = "dataSources.local.xml"
	componentOpen   = `<component name="DataSourceManagerImpl" format="xml" multifile-model="true">`
	localComponent  = `<component name="dataSourceStorageLocal">`
)

func skeleton(component string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  ` + component + `
  </component>
</project>
`
}

// Outcome says what a sync did, so a caller can tell a user why nothing was
// written rather than list the reasons it might have been.
type Outcome int

const (
	// NotJetBrains means the project has no .idea directory.
	NotJetBrains Outcome = iota
	// AlreadyConfigured means a data source already points at the same database.
	AlreadyConfigured
	// Written means lerd's entry was added or brought up to date.
	Written
)

// Sync writes lerd's data source into the project's JetBrains configuration. A
// project with no .idea directory is not open in a JetBrains IDE, and lerd
// creating one would be litter, so it is left alone.
// Sync brings the project's JetBrains configuration in line with the given set
// of connections: lerd's own entries are replaced by exactly these, and every
// other data source in the files is left as it was. The outcome of each source
// is returned in the order it was given.
func Sync(projectDir string, sources []DataSource) ([]Outcome, error) {
	dir := filepath.Join(projectDir, ".idea")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return repeat(NotJetBrains, len(sources)), nil
	}
	body, err := load(dir, dataSourcesFile, componentOpen)
	if err != nil {
		return nil, err
	}
	local, err := load(dir, localFile, localComponent)
	if err != nil {
		return nil, err
	}

	// Entries lerd wrote before are dropped up front, so one whose database the
	// project no longer uses does not linger, and the ones still wanted are
	// written back below.
	keep := map[string]bool{}
	for _, ds := range sources {
		keep[ds.Name] = true
	}
	body = dropOwnedExcept(body, keep)
	local = dropOwnedExcept(local, keep)

	outcomes := make([]Outcome, len(sources))
	for i, ds := range sources {
		uuid := dataSourceUUID(projectDir, ds.Key)
		// Once lerd owns an entry it keeps it current. Until then, a connection
		// the user already wired to the same database is theirs to keep, and a
		// second one beside it would be clutter rather than help.
		if !strings.Contains(body, `uuid="`+uuid+`"`) && hasEquivalent(body, ds.URL) {
			outcomes[i] = AlreadyConfigured
			continue
		}
		body = dropEntry(body, uuid)
		if body, err = insert(body, renderEntry(uuid, ds), componentOpen); err != nil {
			return outcomes, err
		}
		// The user belongs in the sidecar the IDE authenticates from: it takes the
		// user from there, not from the URL, and an entry without one connects as
		// nobody. The password stays in the IDE's own credential store, which is
		// beyond reach, so the first connect asks for it once.
		if ds.User != "" {
			local = dropEntry(local, uuid)
			if local, err = insert(local, renderLocalEntry(uuid, ds), localComponent); err != nil {
				return outcomes, err
			}
		}
		outcomes[i] = Written
	}
	if err := write(dir, dataSourcesFile, body); err != nil {
		return outcomes, err
	}
	return outcomes, write(dir, localFile, local)
}

func repeat(o Outcome, n int) []Outcome {
	out := make([]Outcome, n)
	for i := range out {
		out[i] = o
	}
	return out
}

// dropOwnedExcept removes every entry lerd maintains whose name is not wanted
// any more, which is how a site that changed database loses the old connection.
func dropOwnedExcept(body string, keep map[string]bool) string {
	for {
		name, uuid, found := nextOwnedEntry(body, keep)
		if !found {
			return body
		}
		_ = name
		body = dropEntry(body, uuid)
	}
}

// nextOwnedEntry finds the first lerd-owned entry whose name is no longer
// wanted, returning its name and uuid.
func nextOwnedEntry(body string, keep map[string]bool) (string, string, bool) {
	for rest, offset := body, 0; ; {
		i := strings.Index(rest, "<data-source ")
		if i < 0 {
			return "", "", false
		}
		rest = rest[i:]
		offset += i
		end := strings.Index(rest, ">")
		if end < 0 {
			return "", "", false
		}
		head := rest[:end]
		name, uuid := attr(head, "name"), attr(head, "uuid")
		if Owns(name) && !keep[name] && uuid != "" {
			return name, uuid, true
		}
		rest = rest[len("<data-source "):]
		offset += len("<data-source ")
	}
}

func attr(head, name string) string {
	marker := name + `="`
	i := strings.Index(head, marker)
	if i < 0 {
		return ""
	}
	rest := head[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return unescape(rest[:j])
}

func renderLocalEntry(uuid string, ds DataSource) string {
	return fmt.Sprintf(`    <data-source name="%s" uuid="%s">
      <secret-storage>master_key</secret-storage>
      <user-name>%s</user-name>
    </data-source>
`, escape(ds.Name), uuid, escape(ds.User))
}

// insert places an entry above the closing tag of the component it belongs to,
// adding the component when the file has never held one.
func insert(body, entry, component string) (string, error) {
	if i := lineStartOf(body, "</component>"); i >= 0 {
		return body[:i] + entry + body[i:], nil
	}
	i := lineStartOf(body, "</project>")
	if i < 0 {
		return "", fmt.Errorf("not a JetBrains project file")
	}
	return body[:i] + "  " + component + "\n" + entry + "  </component>\n" + body[i:], nil
}

// Remove drops every entry lerd maintains and leaves everything else as it was.
func Remove(projectDir string) (bool, error) {
	dir := filepath.Join(projectDir, ".idea")
	removed := false
	for _, name := range []string{dataSourcesFile, localFile} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		trimmed := dropOwnedExcept(string(body), nil)
		if trimmed == string(body) {
			continue
		}
		if err := write(dir, name, trimmed); err != nil {
			return removed, err
		}
		removed = true
	}
	return removed, nil
}

// hasEquivalent reports whether the file already holds a data source aimed at
// the same database, however it spells the host.
func hasEquivalent(body, url string) bool {
	for _, found := range jdbcURLs(body) {
		if sameTarget(found, url) {
			return true
		}
	}
	return false
}

func jdbcURLs(body string) []string {
	var out []string
	for rest := body; ; {
		i := strings.Index(rest, "<jdbc-url>")
		if i < 0 {
			return out
		}
		rest = rest[i+len("<jdbc-url>"):]
		j := strings.Index(rest, "</jdbc-url>")
		if j < 0 {
			return out
		}
		out = append(out, strings.TrimSpace(rest[:j]))
		rest = rest[j:]
	}
}

// sameTarget compares two JDBC URLs by what they actually connect to: the
// dialect, the host once loopback is spelled one way, the port and the database.
// Credentials and other query parameters are not part of the target.
func sameTarget(a, b string) bool {
	ah, ap, ad, aok := jdbcTarget(a)
	bh, bp, bd, bok := jdbcTarget(b)
	return aok && bok && ah == bh && ap == bp && ad == bd
}

func jdbcTarget(url string) (dialectHost, port, database string, ok bool) {
	rest, found := strings.CutPrefix(url, "jdbc:")
	if !found {
		return "", "", "", false
	}
	dialect, rest, found := strings.Cut(rest, "://")
	if !found {
		return "", "", "", false
	}
	authority, path, _ := strings.Cut(rest, "/")
	database, _, _ = strings.Cut(path, "?")
	host, port, _ := strings.Cut(authority, ":")
	switch host {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		host = "localhost"
	}
	return dialect + "://" + host, port, database, true
}

// lineStartOf returns the index of the start of the line holding tag, so an
// insertion lands above it and leaves its indentation where it was.
func lineStartOf(body, tag string) int {
	i := strings.Index(body, tag)
	if i < 0 {
		return -1
	}
	for i > 0 && (body[i-1] == ' ' || body[i-1] == '\t') {
		i--
	}
	return i
}

func load(dir, name, component string) (string, error) {
	body, err := os.ReadFile(filepath.Join(dir, name))
	if os.IsNotExist(err) {
		return skeleton(component), nil
	}
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func write(dir, name, body string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
}

// dropEntry removes the data-source block carrying our uuid. The file is edited
// as text rather than parsed and re-marshalled, because everything else in it
// belongs to the user and has to come back byte for byte.
func dropEntry(body, uuid string) string {
	marker := `uuid="` + uuid + `"`
	at := strings.Index(body, marker)
	if at < 0 {
		return body
	}
	start := strings.LastIndex(body[:at], "<data-source ")
	if start < 0 {
		return body
	}
	end := strings.Index(body[at:], "</data-source>")
	if end < 0 {
		return body
	}
	end = at + end + len("</data-source>")
	for end < len(body) && (body[end] == '\n' || body[end] == '\r') {
		end++
	}
	for start > 0 && (body[start-1] == ' ' || body[start-1] == '\t') {
		start--
	}
	return body[:start] + body[end:]
}

func renderEntry(uuid string, ds DataSource) string {
	return fmt.Sprintf(`    <data-source source="LOCAL" name="%s" uuid="%s">
      <driver-ref>%s</driver-ref>
      <synchronize>true</synchronize>
      <jdbc-driver>%s</jdbc-driver>
      <jdbc-url>%s</jdbc-url>
      <working-dir>$ProjectFileDir$</working-dir>
    </data-source>
`, escape(ds.Name), uuid, escape(ds.Driver), escape(ds.Class), escape(ds.URL))
}

var (
	escaper   = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	unescaper = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`)
)

func escape(s string) string   { return escaper.Replace(s) }
func unescape(s string) string { return unescaper.Replace(s) }

// dataSourceUUID derives a stable identifier from the project and the key, so
// repeated runs update the same entries instead of piling up, a site and its
// worktree databases each keep their own, and two projects never collide.
func dataSourceUUID(projectDir, key string) string {
	sum := sha1.Sum([]byte("lerd:datasource:" + projectDir + "\x00" + key))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5, as a name-derived uuid
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	h := hex.EncodeToString(b)
	return strings.Join([]string{h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]}, "-")
}
