package config

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// PackageFetchFunc fetches a package definition from the store and caches it
// locally. version is the package's own major, empty for a package that
// publishes a single unversioned file. The store package registers it at
// startup, the way it does for frameworks, to avoid a circular import.
type PackageFetchFunc func(name, version string) (*FrameworkPackage, error)

var packageFetchHook PackageFetchFunc

// RegisterPackageFetchHook sets the callback used to fetch missing package
// definitions from the store.
func RegisterPackageFetchHook(fn PackageFetchFunc) {
	packageFetchHook = fn
}

// FrameworkPackage is a declaration that belongs to a composer package rather
// than to a framework major. A worker gated on laravel/horizon is horizon's, not
// Laravel 11's, and writing it into every major is what made the same worker
// exist sixteen times over. A project that requires the package gets these
// merged onto whatever framework definition it resolved.
type FrameworkPackage struct {
	// Package is the composer package this file speaks for (vendor/name).
	Package string `yaml:"package"`
	// Version is the package's own major this file targets. A package whose
	// declarations have not changed across its majors publishes one unversioned
	// file and leaves this empty; one whose command moved in a major publishes a
	// file per major, and a project is served the newest at or below the version
	// it has installed.
	Version string `yaml:"version,omitempty"`
	// Frameworks narrows the package to the frameworks, and the ranges of their
	// majors, it applies to. Empty means every framework: a queue driver needs no
	// narrowing, while nativephp/electron is Laravel's alone.
	Frameworks []FrameworkPackageScope    `yaml:"frameworks,omitempty"`
	Workers    map[string]FrameworkWorker `yaml:"workers,omitempty"`
	Commands   []FrameworkCommand         `yaml:"commands,omitempty"`
	// HostCommands are the console commands this package's runtime cannot run
	// inside the container, and the binary that runs them instead.
	HostCommands []HostCommand `yaml:"host_commands,omitempty"`
	// HostBinaries names the executables this package installs that cannot run
	// through the shim at all, matched by name wherever composer put them. A CLI
	// that authenticates over a browser callback binds a loopback listener on a
	// port it picks per run, and the browser dialling 127.0.0.1 on the host
	// reaches nothing, because the listener is in the container's namespace.
	// Unlike HostCommands these belong to no project: composer installs them
	// globally, so there is no framework to resolve them through.
	HostBinaries []string            `yaml:"host_binaries,omitempty"`
	Setup        []FrameworkSetupCmd `yaml:"setup,omitempty"`
	Doctor       *FrameworkDoctor    `yaml:"doctor,omitempty"`
	// Removes takes entries away from the resolved framework, for a major of the
	// package that dropped a command or a worker. Declaring an entry is how a
	// package replaces one, and this is how it deletes one, which it cannot do by
	// staying silent: the copy a declaration was lifted out of is still in the
	// framework's own version files, and silence there means keep it.
	Removes *FrameworkPackageRemoves `yaml:"removes,omitempty"`
}

// GlobalHostBinary reports whether the store declares bin, an executable
// composer installed globally under composerHome, as one that has to run on a
// host PHP. Only packages the global install actually requires are consulted,
// so a bin name never reaches for a definition this machine has no reason to
// hold, and a basename collision between two packages cannot answer for one the
// machine never installed.
func GlobalHostBinary(composerHome, bin string) bool {
	if composerHome == "" || bin == "" {
		return false
	}
	for _, entry := range cachedStorePackages() {
		if !ComposerHasInstalled(composerHome, entry.Name) {
			continue
		}
		pkg := resolveStorePackage(entry, composerHome)
		if pkg != nil && slices.Contains(pkg.HostBinaries, bin) {
			return true
		}
	}
	return false
}

// FrameworkPackageRemoves names entries, by the name each is declared under,
// that this version of the package no longer has.
type FrameworkPackageRemoves struct {
	Workers  []string `yaml:"workers,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Setup    []string `yaml:"setup,omitempty"`
	Doctor   []string `yaml:"doctor,omitempty"`
}

// FrameworkPackageScope binds a package to one framework, optionally to a range
// of its majors. Min and Max are inclusive and either may be omitted for an
// open end, the same shape a framework's PHP range uses.
type FrameworkPackageScope struct {
	Name string `yaml:"name"`
	Min  string `yaml:"min,omitempty"`
	Max  string `yaml:"max,omitempty"`
}

// AppliesTo reports whether the package's declarations belong on fw.
func (p *FrameworkPackage) AppliesTo(fw *Framework) bool {
	if p == nil || fw == nil {
		return false
	}
	if len(p.Frameworks) == 0 {
		return true
	}
	version := fw.Version
	if fw.DetectedVersion != "" {
		version = fw.DetectedVersion
	}
	for _, scope := range p.Frameworks {
		if scope.Name != "" && scope.Name != fw.Name {
			continue
		}
		if scope.covers(version) {
			return true
		}
	}
	return false
}

// covers reports whether a framework major falls inside the scope's range. A
// range is stated against a major, so a version that is not one (a project whose
// version never resolved) matches only an unbounded scope.
func (s FrameworkPackageScope) covers(version string) bool {
	if s.Min == "" && s.Max == "" {
		return true
	}
	v, err := strconv.Atoi(version)
	if err != nil {
		return false
	}
	if lo, err := strconv.Atoi(s.Min); err == nil && v < lo {
		return false
	}
	if hi, err := strconv.Atoi(s.Max); err == nil && v > hi {
		return false
	}
	return true
}

// mergeStorePackages folds the store's package layer onto a resolved framework:
// every package the store publishes that this project requires, and that claims
// this framework, contributes its workers, commands and doctor checks.
func mergeStorePackages(fw *Framework, projectDir string) *Framework {
	if fw == nil || projectDir == "" {
		return fw
	}
	for _, entry := range cachedStorePackages() {
		if !ComposerHasInstalled(projectDir, entry.Name) {
			continue
		}
		pkg := resolveStorePackage(entry, projectDir)
		if pkg == nil || !pkg.AppliesTo(fw) {
			continue
		}
		applyPackage(fw, pkg)
	}
	return fw
}

// resolveStorePackage returns the definition serving this project, falling back
// down the published versions when the one it should have cannot be had.
//
// The day the store starts publishing a major of its own for a package that had
// one file, every install is asked for a file it has never fetched. Online that
// is one fetch and it is cached. Offline it is nothing at all, and the worker
// this machine has been running for months would disappear until the network
// came back, so the newest cached file at or below the wanted version answers
// instead: an older description of the package beats none.
func resolveStorePackage(entry StorePackageEntry, projectDir string) *FrameworkPackage {
	want := pickPackageVersion(projectDir, entry)
	if pkg := loadStorePackage(entry.Name, want); pkg != nil {
		return pkg
	}
	for _, v := range olderPublishedVersions(entry, want) {
		if pkg := loadPackageYAML(StorePackageFile(entry.Name, v)); pkg != nil {
			return pkg
		}
	}
	return nil
}

// olderPublishedVersions lists the versions to try after want, newest first and
// the unversioned file last. Only the cache is consulted for these, so a store
// this machine cannot reach costs one failed fetch rather than one per version.
func olderPublishedVersions(entry StorePackageEntry, want string) []string {
	ceiling, err := strconv.Atoi(want)
	if err != nil {
		return nil // want is the unversioned file already, nothing below it
	}
	majors := sortedMajors(entry.Versions)
	var out []string
	for i := len(majors) - 1; i >= 0; i-- {
		if majors[i] < ceiling {
			out = append(out, strconv.Itoa(majors[i]))
		}
	}
	return append(out, "")
}

// applyPackage merges one package's declarations onto fw. The package wins every
// name collision: it is the newer statement about the entry and the one place it
// is now maintained, so a copy left behind in a version file is replaced rather
// than shadowing it. A collision is replaced in place, keeping the order the
// definition listed its commands and checks in. The user's overlay and a
// project's .lerd.yaml are merged after this and still sit above it.
func applyPackage(fw *Framework, pkg *FrameworkPackage) {
	if len(pkg.Workers) > 0 && fw.Workers == nil {
		fw.Workers = make(map[string]FrameworkWorker, len(pkg.Workers))
	}
	for name, w := range pkg.Workers {
		fw.Workers[name] = w
	}
	for _, cmd := range pkg.Commands {
		fw.Commands = upsertCommand(fw.Commands, cmd)
	}
	// Prepended, so a package's routing is consulted before a framework file's
	// and the first match still wins.
	fw.HostCommands = append(append([]HostCommand(nil), pkg.HostCommands...), fw.HostCommands...)
	// A setup step has no name to key on, so the command it runs is its identity:
	// the framework files still carry the copies this package was lifted out of,
	// and matching on the command is what keeps a project from being offered the
	// same migration twice.
	for _, step := range pkg.Setup {
		if !hasSetupCommand(fw.Setup, step.Command) {
			fw.Setup = append(fw.Setup, step)
		}
	}
	if pkg.Doctor != nil && len(pkg.Doctor.Checks) > 0 {
		if fw.Doctor == nil {
			fw.Doctor = &FrameworkDoctor{}
		}
		for _, check := range pkg.Doctor.Checks {
			fw.Doctor.Checks = upsertDoctorCheck(fw.Doctor.Checks, check)
		}
	}
	applyPackageRemovals(fw, pkg.Removes)
}

// applyPackageRemovals drops the entries a version of the package says it no
// longer has. A setup step is named by the label it is listed under, the only
// name it has.
func applyPackageRemovals(fw *Framework, rm *FrameworkPackageRemoves) {
	if rm == nil {
		return
	}
	for _, name := range rm.Workers {
		delete(fw.Workers, name)
	}
	for _, name := range rm.Commands {
		fw.Commands = slices.DeleteFunc(fw.Commands, func(c FrameworkCommand) bool { return c.Name == name })
	}
	for _, label := range rm.Setup {
		fw.Setup = slices.DeleteFunc(fw.Setup, func(s FrameworkSetupCmd) bool { return s.Label == label })
	}
	if fw.Doctor == nil {
		return
	}
	for _, name := range rm.Doctor {
		fw.Doctor.Checks = slices.DeleteFunc(fw.Doctor.Checks, func(c DoctorCheck) bool { return c.Name == name })
	}
}

func upsertCommand(cmds []FrameworkCommand, cmd FrameworkCommand) []FrameworkCommand {
	for i := range cmds {
		if cmds[i].Name == cmd.Name {
			cmds[i] = cmd
			return cmds
		}
	}
	return append(cmds, cmd)
}

func hasSetupCommand(steps []FrameworkSetupCmd, command string) bool {
	for _, s := range steps {
		if s.Command == command {
			return true
		}
	}
	return false
}

func upsertDoctorCheck(checks []DoctorCheck, check DoctorCheck) []DoctorCheck {
	for i := range checks {
		if checks[i].Name == check.Name {
			checks[i] = check
			return checks
		}
	}
	return append(checks, check)
}

// composerPackageName is what a path built from an index entry is allowed to
// look like. The index is remote data and the name becomes a file path, so
// anything with a second slash, a leading dot, or an upper-case letter is
// refused rather than written outside the packages directory.
var composerPackageName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*/[a-z0-9][a-z0-9._-]*$`)

// PackageSlug is a composer package as one file name, vendor and package joined
// by the dash they are already made of. Empty when the name is not one composer
// could publish, which is what keeps a name read out of the remote index from
// naming a path of its own.
func PackageSlug(name string) string {
	if !composerPackageName.MatchString(name) {
		return ""
	}
	return strings.ReplaceAll(name, "/", "-")
}

// StorePackageFile returns the local cache path for a package definition, or
// empty when the name is not one composer could publish. version is the
// package's own major; empty is the unversioned file.
func StorePackageFile(name, version string) string {
	slug := PackageSlug(name)
	if slug == "" {
		return ""
	}
	if version != "" {
		slug += "@" + version
	}
	return filepath.Join(StorePackagesDir(), slug+".yaml")
}

// pickPackageVersion decides which published file serves the version of the
// package a project has installed. Every package has one unversioned file, and a
// major that changes what lerd runs adds a file of its own, which serves that
// major and every later one until the next such file. So the answer is the
// newest published version at or below what the project installed, and the
// unversioned file below the first of them: a package that has never broken
// anything stays one file, and one that has keeps serving older projects from
// the file that was right for them.
//
// A constraint no major can be read out of (dev-main, *) takes the latest, since
// a project tracking a branch is on the newest thing published.
func pickPackageVersion(projectDir string, entry StorePackageEntry) string {
	if len(entry.Versions) == 0 {
		return ""
	}
	detected, err := strconv.Atoi(installedMajor(projectDir, entry.Name))
	if err != nil {
		return entry.Latest
	}
	best := ""
	for _, v := range sortedMajors(entry.Versions) {
		if v <= detected {
			best = strconv.Itoa(v)
		}
	}
	return best
}

// sortedMajors returns the numeric versions in ascending order, dropping any
// that is not a major.
func sortedMajors(versions []string) []int {
	var out []int
	for _, v := range versions {
		if n, err := strconv.Atoi(v); err == nil {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// loadStorePackage returns the cached definition for a package, fetching it when
// the cache is missing or a day old, exactly like a framework definition.
func loadStorePackage(name, version string) *FrameworkPackage {
	path := StorePackageFile(name, version)
	if path == "" {
		return nil
	}
	pkg := loadPackageYAML(path)
	if packageFetchHook == nil {
		return pkg
	}
	if pkg != nil && !olderThan(path, storeRefreshWindow) {
		return pkg
	}
	if fetched, err := packageFetchHook(name, version); err == nil && fetched != nil {
		return fetched
	}
	return pkg
}

func loadPackageYAML(path string) *FrameworkPackage {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg FrameworkPackage
	if yaml.Unmarshal(data, &pkg) != nil || pkg.Package == "" {
		return nil
	}
	return &pkg
}

// SaveStorePackage caches a fetched package definition.
func SaveStorePackage(pkg *FrameworkPackage) error {
	path := StorePackageFile(pkg.Package, pkg.Version)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(StorePackagesDir(), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(pkg)
	if err != nil {
		return err
	}
	return publishStoreFile(path, data, 0644)
}

// StorePackageInfo is one published package as a lister sees it: what the
// cached file declares, which file serves this project, and whether the project
// requires the package at all.
type StorePackageInfo struct {
	Name string
	// Version is the file that would serve this project, empty for the
	// unversioned one. Cached reports whether that file is on disk.
	Version  string
	Cached   bool
	Required bool
	Workers  []string
	Commands []string
	Setup    int
	Doctor   int
	// HostBinaries are the globally installed binaries the package routes out of
	// the container, the one declaration that contributes nothing to a framework
	// and would otherwise leave the package looking empty in a listing.
	HostBinaries []string
}

// ListStorePackages describes the package layer for projectDir, which may be
// empty for a listing outside a project. It reads the cache and never fetches:
// a listing that pulled sixteen files from the network would be a different
// command than the one people run to see what they already have.
func ListStorePackages(projectDir string) []StorePackageInfo {
	entries := cachedStorePackages()
	out := make([]StorePackageInfo, 0, len(entries))
	for _, entry := range entries {
		info := StorePackageInfo{Name: entry.Name}
		if projectDir != "" {
			info.Required = ComposerHasInstalled(projectDir, entry.Name)
		}
		info.Version = pickPackageVersion(projectDir, entry)
		if pkg := loadPackageYAML(StorePackageFile(entry.Name, info.Version)); pkg != nil {
			info.Cached = true
			for name := range pkg.Workers {
				info.Workers = append(info.Workers, name)
			}
			sort.Strings(info.Workers)
			for _, c := range pkg.Commands {
				info.Commands = append(info.Commands, c.Name)
			}
			info.HostBinaries = pkg.HostBinaries
			info.Setup = len(pkg.Setup)
			if pkg.Doctor != nil {
				info.Doctor = len(pkg.Doctor.Checks)
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		return out[i].Name < out[j].Name
	})
	return out
}
