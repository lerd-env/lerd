package plugins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
)

// CatalogURL is the approved plugin listing plugins.lerd.sh publishes from the
// lerd-env/plugins registry. A var so tests can point it at their own server.
var CatalogURL = "https://plugins.lerd.sh/index.json"

// catalogMaxAge is how long a fetched catalog is trusted before it is fetched again.
const catalogMaxAge = 24 * time.Hour

// Catalog is the registry's published index.
type Catalog struct {
	Plugins []Listing `json:"plugins"`
}

// Listing is one approved plugin: its repository and the exact commit a
// maintainer approved, which is the only one lerd will install.
type Listing struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Repo        string   `json:"repo"`
	Commit      string   `json:"commit"`
	Author      string   `json:"author,omitempty"`
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Lerd is the approved commit's own `lerd` constraint.
	Lerd string `json:"lerd,omitempty"`
	// Requires is what the approved commit needs on the machine, as
	// Plugin.Requirements reads it.
	Requires []string `json:"requires,omitempty"`
	// Images are copied by the registry at the approved commit and published
	// beside the index, the first one being the preview. The index lists them
	// relative to itself; LoadCatalog hands them out as absolute URLs.
	Images []string `json:"images,omitempty"`
	// AddedAt and UpdatedAt are the days the plugin was first approved and its
	// current commit was; Stars is its repository's count when the index was built.
	AddedAt   string `json:"added_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Stars     int    `json:"stars,omitempty"`
}

// Source records which listing and commit an installed plugin came from, kept
// in the plugin's folder so it goes with the install.
type Source struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
}

const sourceFile = ".lerd-source.json"

var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

func catalogCachePath() string { return filepath.Join(config.DataDir(), "plugin-catalog.json") }

// LoadCatalog returns the approved plugins, fetching the index when the cached
// copy is a day old. When the fetch fails a cached copy, however old, still
// answers, so being offline never hides what is already known.
func LoadCatalog() (*Catalog, error) {
	if info, err := os.Stat(catalogCachePath()); err == nil && time.Since(info.ModTime()) < catalogMaxAge {
		if c, err := readCatalogCache(); err == nil {
			return c, nil
		}
	}
	if err := refreshCatalog(); err != nil {
		if c, cacheErr := readCatalogCache(); cacheErr == nil {
			return c, nil
		}
		return nil, err
	}
	return readCatalogCache()
}

func refreshCatalog() error {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(CatalogURL)
	if err != nil {
		return fmt.Errorf("fetching the plugin catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching the plugin catalog: %s", resp.Status)
	}
	var c Catalog
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return fmt.Errorf("reading the plugin catalog: %w", err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	path := catalogCachePath()
	config.GuardRealWrite(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Always rewritten, even unchanged: the file's age is what says it is fresh.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readCatalogCache() (*Catalog, error) {
	data, err := os.ReadFile(catalogCachePath())
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.Plugins = validListings(c.Plugins)
	return &c, nil
}

// validListings drops any listing lerd could not install safely or draw,
// rather than letting one bad entry take the catalog down.
func validListings(in []Listing) []Listing {
	out := make([]Listing, 0, len(in))
	for _, l := range in {
		if !nameRe.MatchString(l.Name) || !commitRe.MatchString(l.Commit) {
			continue
		}
		if !strings.HasPrefix(l.Repo, "https://") && !strings.HasPrefix(l.Repo, "file://") {
			continue
		}
		images, ok := resolveImages(l.Images)
		if ok {
			l.Images = images
			out = append(out, l)
		}
	}
	return out
}

// resolveImages turns the index's image paths, relative to the index itself,
// into absolute URLs. A path with a scheme, a host or a way up is refused, so
// an image can only ever come from where the index is published.
func resolveImages(paths []string) ([]string, bool) {
	base, err := url.Parse(CatalogURL)
	if err != nil {
		return nil, false
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" || strings.ContainsAny(p, ":\\") || strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
			return nil, false
		}
		ref, err := url.Parse(p)
		if err != nil {
			return nil, false
		}
		out = append(out, base.ResolveReference(ref).String())
	}
	return out, true
}

// FindListing returns the approved listing called name.
func FindListing(name string) (*Listing, error) {
	c, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	for i := range c.Plugins {
		if c.Plugins[i].Name == name {
			return &c.Plugins[i], nil
		}
	}
	return nil, fmt.Errorf("plugin %s is not in the plugin catalog", name)
}

// InstallListed installs an approved plugin at the commit it was approved at.
func InstallListed(name string) (*Plugin, error) {
	return installListing(name, false)
}

// UpdateListed replaces an installed plugin with its currently approved commit.
// The data it stored is kept.
func UpdateListed(name string) error {
	if _, err := os.Stat(filepath.Join(Dir(), name, ManifestFile)); err != nil {
		return fmt.Errorf("plugin %s is not installed", name)
	}
	_, err := installListing(name, true)
	return err
}

func installListing(name string, replace bool) (*Plugin, error) {
	l, err := FindListing(name)
	if err != nil {
		return nil, err
	}
	stage, err := newStage()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "plugin")
	if err := fetchCommit(l.Repo, l.Commit, staged); err != nil {
		return nil, err
	}
	return place(stage, staged, l.Name, replace, &Source{Repo: l.Repo, Commit: l.Commit})
}

// fetchCommit checks out exactly commit of repo into dir and drops the git
// history, so what is installed is the approved snapshot and nothing else.
func fetchCommit(repo, commit, dir string) error {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		// Fail on a private repo rather than wait on a password prompt no one
		// can answer when the dashboard asked for the install.
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if out, err := run("init", "-q", dir); err != nil {
		return fmt.Errorf("git init: %s", out)
	}
	if out, err := run("-C", dir, "fetch", "-q", "--depth", "1", repo, commit); err != nil {
		return fmt.Errorf("fetching %s at %s: %s", repo, commit, out)
	}
	if out, err := run("-C", dir, "checkout", "-q", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("checking out %s: %s", commit, out)
	}
	head, err := run("-C", dir, "rev-parse", "HEAD")
	if err != nil || head != commit {
		return fmt.Errorf("checked out %s, not the approved %s", head, commit)
	}
	return os.RemoveAll(filepath.Join(dir, ".git"))
}

// IsCatalogName reports whether arg names a catalog plugin rather than a
// folder or a repository, which is how `install` tells them apart.
func IsCatalogName(arg string) bool {
	if _, err := os.Stat(arg); err == nil {
		return false
	}
	return nameRe.MatchString(arg)
}

// PendingUpdate returns the newer approved listing for an installed catalog
// plugin, or false when it is current, unlisted, needs a newer lerd or the
// catalog is unreachable.
func PendingUpdate(name string) (*Listing, bool) {
	src, ok := InstalledSource(name)
	if !ok {
		return nil, false
	}
	l, err := FindListing(name)
	if err != nil || l.Commit == src.Commit || !Compatible(l.Lerd) {
		return nil, false
	}
	return l, true
}

// InstalledSource reports the listing commit an installed plugin came from;
// false for one installed from a folder or an unlisted repository.
func InstalledSource(name string) (Source, bool) {
	var s Source
	data, err := os.ReadFile(filepath.Join(Dir(), name, sourceFile))
	if err != nil || json.Unmarshal(data, &s) != nil {
		return Source{}, false
	}
	return s, true
}

func writeSource(dir string, s *Source) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sourceFile), data, 0o644)
}
