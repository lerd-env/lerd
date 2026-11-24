package config

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
)

// ForeignDBHost is the database server a project's env names when it is not one
// lerd runs or has been told about. On loopback the login decides, not the port:
// a moved lerd service leaves its old port behind for the server that took it.
func ForeignDBHost(projectDir string) (string, bool) {
	if ExternalDBServiceFor(projectDir) != nil || onFileDB(projectDir) {
		return "", false
	}
	file, format := envFileOrSeed(projectDir)
	e := dbEndpointIn(projectDir, dbBindingsIn(frameworkFor(projectDir), file, format), file, format)
	e.Host = strings.Trim(strings.TrimSpace(e.Host), `"'`)
	if e.Host == "" || strings.HasPrefix(e.Host, "lerd-") {
		return "", false
	}
	if isLoopbackHost(e.Host) && hasLerdDBLogin(projectDir) {
		return "", false
	}
	if e.Port == "" {
		return e.Host, true
	}
	return net.JoinHostPort(e.Host, e.Port), true
}

// envFileOrSeed is the env file a project has, or the one `lerd env` will seed it
// from while it has none, which Symfony reads as its own in the meantime.
func envFileOrSeed(projectDir string) (file, format string) {
	file, format = EnvFileFor(projectDir)
	fw := frameworkFor(projectDir)
	if fw == nil || fw.Env.ExampleFile == "" {
		return file, format
	}
	if _, err := os.Stat(filepath.Join(projectDir, file)); os.IsNotExist(err) {
		return fw.Env.ExampleFile, format
	}
	return file, format
}

// onFileDB reports whether a project is on a file database, whose env can still
// hold the host of a server it has left.
func onFileDB(projectDir string) bool {
	file, format := envFileOrSeed(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	if fw := frameworkFor(projectDir); fw != nil && fw.Env.SQLite != nil && DetectRulesMatch(fw.Env.SQLite.Detect, vals) {
		return true
	}
	return !serverDBDriver(vals["DB_CONNECTION"])
}

// hasLerdDBLogin reports whether a project's env logs in with what lerd writes for
// one of the databases its framework supports, or with the developer's own override.
// A definition naming no login leaves only the published port to go by.
func hasLerdDBLogin(projectDir string) bool {
	file, format := envFileOrSeed(projectDir)
	vals := envfile.Values(filepath.Join(projectDir, file), format)
	overrides, _ := envfile.ReadOverride(projectDir)
	// A checkout holds its parent's login without the file that sets it.
	if parent, ok := ParentSiteForWorktreeDir(projectDir); ok && len(overrides) == 0 {
		overrides, _ = envfile.ReadOverride(parent.Path)
	}
	declared := false
	blocks := lerdDBVars(projectDir)
	for _, vars := range blocks {
		matches, hasLogin := loginMatches(vars, vals, overrides)
		if matches {
			return true
		}
		declared = declared || hasLogin
	}
	if declared {
		return false
	}
	for service := range blocks {
		if _, external := ExternalDBHost(projectDir, service); !external {
			return true
		}
	}
	return false
}

// lerdDBVars is the env block lerd writes for each database a project may be on.
func lerdDBVars(projectDir string) map[string][]string {
	out := map[string][]string{}
	if fw := frameworkFor(projectDir); fw != nil {
		for name, def := range fw.Env.Services {
			if name != "sqlite" && IsDBServiceName(name) {
				out[name] = def.Vars
			}
		}
	}
	if len(out) == 0 {
		return map[string][]string{"mysql": DefaultPresetEnvVars("mysql"), "postgres": DefaultPresetEnvVars("postgres")}
	}
	return out
}

// loginMatches compares the user and password a vars block declares, as keys or
// inside a URL, with a project's values. hasLogin is whether it declares any.
func loginMatches(vars []string, vals, overrides map[string]string) (matches, hasLogin bool) {
	matches = true
	for _, kv := range vars {
		k, want, _ := strings.Cut(kv, "=")
		got := vals[k]
		upper := strings.ToUpper(k)
		switch {
		case strings.Contains(want, "://"):
			got, want = urlLogin(got), urlLogin(want)
		case strings.Contains(upper, "USER"), strings.Contains(upper, "PASS"):
			if v, ok := overrides[k]; ok {
				want = v
			}
		default:
			continue
		}
		if strings.Trim(got, `"'`) != strings.Trim(want, `"'`) {
			matches = false
		}
		hasLogin = true
	}
	return matches && hasLogin, hasLogin
}

func urlLogin(raw string) string {
	u, err := url.Parse(strings.Trim(raw, `"'`))
	if err != nil || u.User == nil {
		return ""
	}
	return u.User.String()
}
