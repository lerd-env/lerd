package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"
)

// sitesBackupKeep is how many previous versions of sites.yaml are kept. Only a
// save that changes the file pushes one in, so ten covers days of ordinary use
// while staying small enough to never matter on disk.
const sitesBackupKeep = 10

// A backup is named for the second it was taken plus a counter, so the names
// sort in the order the backups were taken even when a save follows a save.
const sitesBackupTimeLayout = "20060102-150405"

var sitesBackupRe = regexp.MustCompile(`\Asites-(\d{8}-\d{6})\.(\d{3})\.yaml\z`)

// SitesBackup is one kept registry. Sites is the number of sites it holds,
// which is what tells a good backup from the one taken right after a wipe.
type SitesBackup struct {
	Name  string    `json:"name"`
	Time  time.Time `json:"time"`
	Sites int       `json:"sites"`
}

var sitesBackupMu sync.Mutex

// backupSitesFile preserves the current sites.yaml before next is written over
// it. A save that changes nothing is skipped, which matters because the daemon
// rewrites the registry constantly (idle state, worker toggles, version
// refreshes) and that churn would otherwise push the last real edit out of the
// window within minutes.
//
// Every failure here is silent on purpose: a safety net that fails must not
// take the save down with it.
func backupSitesFile(next []byte) {
	// Two writers picking the same free name would leave one version behind.
	sitesBackupMu.Lock()
	defer sitesBackupMu.Unlock()

	current, err := os.ReadFile(SitesFile())
	if err != nil || len(current) == 0 || bytes.Equal(current, next) {
		return
	}
	if newest, err := newestSitesBackup(); err == nil && bytes.Equal(newest, current) {
		return
	}
	if err := os.MkdirAll(SitesBackupDir(), 0755); err != nil {
		return
	}
	path, err := uniqueSitesBackupPath(time.Now())
	if err != nil {
		return
	}
	if err := writeFileAtomic(path, current, 0644); err != nil {
		return
	}
	pruneSitesBackups()
}

// uniqueSitesBackupPath names the next backup for this second: one past the
// highest counter already there, never a freed one, so the names keep sorting
// in the order the backups were taken even after a prune.
func uniqueSitesBackupPath(now time.Time) (string, error) {
	stamp := now.Format(sitesBackupTimeLayout)
	names, err := listSitesBackupNames()
	if err != nil {
		return "", err
	}
	next := 0
	for _, name := range names {
		m := sitesBackupRe.FindStringSubmatch(name)
		if m[1] != stamp {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		if n+1 > next {
			next = n + 1
		}
	}
	if next > 999 {
		return "", fmt.Errorf("backup name space exhausted for %s", stamp)
	}
	return filepath.Join(SitesBackupDir(), fmt.Sprintf("sites-%s.%03d.yaml", stamp, next)), nil
}

func newestSitesBackup() ([]byte, error) {
	list, err := listSitesBackupNames()
	if err != nil || len(list) == 0 {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, err
	}
	return os.ReadFile(filepath.Join(SitesBackupDir(), list[0]))
}

// listSitesBackupNames returns the backup file names, newest first.
func listSitesBackupNames() ([]string, error) {
	entries, err := os.ReadDir(SitesBackupDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && sitesBackupRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

func pruneSitesBackups() {
	names, err := listSitesBackupNames()
	if err != nil {
		return
	}
	for _, name := range names[min(len(names), sitesBackupKeep):] {
		os.Remove(filepath.Join(SitesBackupDir(), name)) //nolint:errcheck
	}
}

// ListSitesBackups returns the kept registries, newest first.
func ListSitesBackups() ([]SitesBackup, error) {
	names, err := listSitesBackupNames()
	if err != nil {
		return nil, err
	}
	out := make([]SitesBackup, 0, len(names))
	for _, name := range names {
		reg, err := ReadSitesBackup(name)
		if err != nil {
			continue
		}
		stamp, _ := time.ParseInLocation(sitesBackupTimeLayout, sitesBackupRe.FindStringSubmatch(name)[1], time.Local)
		out = append(out, SitesBackup{Name: name, Time: stamp, Sites: len(reg.Sites)})
	}
	return out, nil
}

// ReadSitesBackup parses one backup. The name is validated first, so a caller
// can pass it straight through from a CLI argument or an API request.
func ReadSitesBackup(name string) (*SiteRegistry, error) {
	if !sitesBackupRe.MatchString(name) {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(SitesBackupDir(), name))
	if err != nil {
		return nil, err
	}
	return decodeSiteRegistry(data)
}

// RestoreSitesBackup writes a kept registry back over sites.yaml, returning it
// with the name it came from. An empty name takes the newest. The registry it
// replaces is backed up by the save itself, so restoring the wrong one is
// undone by restoring again.
func RestoreSitesBackup(name string) (*SiteRegistry, string, error) {
	siteWriteMu.Lock()
	defer siteWriteMu.Unlock()

	names, err := listSitesBackupNames()
	if err != nil {
		return nil, "", err
	}
	if len(names) == 0 {
		return nil, "", fmt.Errorf("no sites.yaml backup available")
	}
	if name == "" {
		name = names[0]
	} else if !slices.Contains(names, name) {
		return nil, "", fmt.Errorf("no such backup: %s", name)
	}
	reg, err := ReadSitesBackup(name)
	if err != nil {
		return nil, "", fmt.Errorf("reading backup %s: %w", name, err)
	}
	if err := SaveSites(reg); err != nil {
		return nil, "", err
	}
	return reg, name, nil
}
