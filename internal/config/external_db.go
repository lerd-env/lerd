package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExternalDB marks a service as a database server lerd does not run. There is no
// container behind it: the machine's own clients in Tools talk to the server, and
// the password lives only in 0600 files.
type ExternalDB struct {
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`
	User    string `yaml:"user" json:"user"`
	Tools   string `yaml:"tools" json:"tools"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// LocalHost is the server as this machine reaches it, where the name containers
// use for it does not resolve.
func (e ExternalDB) LocalHost() string {
	if e.Host == "host.containers.internal" {
		return "127.0.0.1"
	}
	return e.Host
}

// Address is the server as people read it.
func (e ExternalDB) Address() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// ExternalDBFamily is the engine family a preset speaks to, mysql or postgres,
// or "" for one this feature does not cover.
func ExternalDBFamily(family string) string {
	switch family {
	case "mysql", "mariadb":
		return "mysql"
	case "postgres":
		return "postgres"
	}
	return ""
}

// ExternalDBClientContent renders the credentials in the family's own format.
func ExternalDBClientContent(family string, e ExternalDB, password string) string {
	if ExternalDBFamily(family) == "postgres" {
		return fmt.Sprintf("%s:%d:*:%s:%s\n", pgpassField(e.LocalHost()), e.Port, pgpassField(e.User), pgpassField(password))
	}
	return fmt.Sprintf("[client]\nhost=%s\nport=%d\nuser=%s\npassword=\"%s\"\n", e.LocalHost(), e.Port, e.User, cnfValue(password))
}

func pgpassField(v string) string {
	return strings.NewReplacer(`\`, `\\`, `:`, `\:`).Replace(v)
}

func cnfValue(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v)
}

// clearLerdPassword drops any password the environment carries, which would
// otherwise beat the client file.
func clearLerdPassword(cmd string) string {
	if strings.TrimSpace(cmd) == "" {
		return cmd
	}
	return "unset MYSQL_PWD PGPASSWORD; " + cmd
}

var lerdClientFlags = regexp.MustCompile(`\s+(-h\s*127\.0\.0\.1|-uroot|-plerd|-U\s*postgres)\b`)

// ExternalIntrospect is a preset's database commands without the flags naming
// lerd's own server, so clients read the external one from their client file. A
// command still naming it is refused, since a store preset may spell these differently.
func ExternalIntrospect(in *Introspect) (*Introspect, error) {
	if in == nil {
		return nil, fmt.Errorf("preset declares no database commands")
	}
	raw, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	stripped := lerdClientFlags.ReplaceAllString(string(raw), "")
	for _, left := range []string{"127.0.0.1", "-uroot", "-plerd", "-U postgres"} {
		if strings.Contains(stripped, left) {
			return nil, fmt.Errorf("preset command still names %s, so it cannot run against an external server", left)
		}
	}
	var out Introspect
	if err := yaml.Unmarshal([]byte(stripped), &out); err != nil {
		return nil, err
	}
	for i := range out.Entities {
		e := &out.Entities[i]
		e.List = clearLerdPassword(e.List)
		for k, a := range e.Actions {
			a.Exec = clearLerdPassword(a.Exec)
			e.Actions[k] = a
		}
	}
	return &out, nil
}

// ExternalDBPasswordFile keeps the password on its own, for the sites that need it
// in their env; only the user can read it, like the client file beside it.
func ExternalDBPasswordFile(name string) string {
	return filepath.Join(ServiceFilesDir(name), "external-password")
}

var urlCredentials = regexp.MustCompile(`://[^/@]*@[^/?]*`)

// ExternalDBVars points a service's env vars at the external server. It goes by
// what each key is, not by a framework's spelling of it, so a Laravel DB_HOST, a
// Symfony DATABASE_URL and a dotted PHP config key all land the same way.
func ExternalDBVars(vars []string, e ExternalDB, host, password string) []string {
	// WordPress and others have no port key, so the host has to carry it.
	hostValue := net.JoinHostPort(host, strconv.Itoa(e.Port))
	for _, kv := range vars {
		k, _, _ := strings.Cut(kv, "=")
		if strings.Contains(strings.ToUpper(k), "PORT") {
			hostValue = host
		}
	}
	out := make([]string, 0, len(vars))
	for _, kv := range vars {
		k, v, _ := strings.Cut(kv, "=")
		key := strings.ToUpper(k)
		switch {
		case strings.Contains(v, "://"):
			v = urlCredentials.ReplaceAllString(v, "://"+url.UserPassword(e.User, password).String()+"@"+net.JoinHostPort(host, strconv.Itoa(e.Port)))
		case strings.Contains(key, "HOST"):
			v = hostValue
		case strings.Contains(key, "PORT"):
			v = strconv.Itoa(e.Port)
		case strings.Contains(key, "USER"):
			v = e.User
		case strings.Contains(key, "PASS"):
			v = password
		}
		out = append(out, k+"="+v)
	}
	return out
}

// ExternalDBServiceFor is the external service a project chose in .lerd.yaml, when
// its env still names that server. Loopback counts for a site whose PHP runs on
// the machine, and a host the user changed by hand no longer matches.
func ExternalDBServiceFor(projectDir string) *CustomService {
	if svc := externalDBPicked(projectDir, projectDir); svc != nil {
		return svc
	}
	// db:use writes .lerd.yaml in the site's folder, which a worktree does not get
	// until it is committed.
	if parent, ok := ParentSiteForWorktreeDir(projectDir); ok {
		return externalDBPicked(parent.Path, projectDir)
	}
	return nil
}

// externalDBPicked is the external service chooser's .lerd.yaml names, when
// projectDir's env still points at that server.
func externalDBPicked(chooser, projectDir string) *CustomService {
	proj, err := LoadProjectConfig(chooser)
	if err != nil {
		return nil
	}
	for _, picked := range proj.Services {
		svc, err := LoadCustomService(picked.Name)
		if err == nil && svc.External != nil && ExternalDBEnvMatches(projectDir, svc) {
			return svc
		}
	}
	return nil
}

// ExternalDBEnvMatches reports whether a checkout's env still names the server.
func ExternalDBEnvMatches(projectDir string, svc *CustomService) bool {
	e := DBEndpointFor(projectDir)
	sameHost := e.Host == svc.External.Host || (svc.External.Host == "host.containers.internal" && isLoopbackHost(e.Host))
	return sameHost && (e.Port == "" || atoiOr(e.Port) == svc.External.Port)
}
