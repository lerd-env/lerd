package serviceops

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// ExternalRequest is a database server lerd does not run. Engine is the preset
// whose commands speak to it, Tools the folder holding its clients, found on the
// PATH when empty.
type ExternalRequest struct {
	Name, Engine, Version, Host string
	Port                        int
	User, Password, Tools       string
}

// containerHost is how a container reaches this machine, which is where a user
// typing 127.0.0.1 means: inside the container that is the container itself.
func containerHost(host string) string {
	switch strings.Trim(host, "[]") {
	case "127.0.0.1", "localhost", "::1":
		return "host.containers.internal"
	}
	return host
}

// externalService turns a preset into the service for a server.
func externalService(req ExternalRequest) (*config.CustomService, error) {
	preset, err := config.EnsurePreset(req.Engine)
	if err != nil {
		return nil, err
	}
	base, err := preset.Resolve(req.Version)
	if err != nil {
		return nil, err
	}
	family := config.FamilyOf(base)
	if config.ExternalDBFamily(family) == "" {
		return nil, fmt.Errorf("%s is not a MySQL, MariaDB or PostgreSQL engine", req.Engine)
	}
	introspect, err := config.ExternalIntrospect(base.Introspect)
	if err != nil {
		return nil, err
	}
	tools, err := externalToolsDir(family, req.Tools)
	if err != nil {
		return nil, err
	}
	ext := config.ExternalDB{Host: containerHost(req.Host), Port: req.Port, User: req.User, Tools: tools}
	return &config.CustomService{
		Name:        req.Name,
		Family:      family,
		Label:       req.Name,
		Description: fmt.Sprintf("External %s at %s:%d", req.Engine, ext.Host, ext.Port),
		Category:    base.Category,
		Icon:        base.Icon,
		Color:       base.Color,
		Introspect:  introspect,
		EnvVars:     base.EnvVars,
		External:    &ext,
	}, nil
}

// externalToolsDir is the folder a server's clients run from: the one the user
// named, which has to hold them, or the first on the PATH that does.
func externalToolsDir(family, dir string) (string, error) {
	if dir = strings.TrimSpace(dir); dir != "" {
		if missing := config.MissingExternalTools(family, dir); len(missing) > 0 {
			return "", fmt.Errorf("%s has no %s", dir, strings.Join(missing, " or "))
		}
		return dir, nil
	}
	if found := config.LocateExternalTools(family, externalSearchDirs()); found != "" {
		return found, nil
	}
	return "", fmt.Errorf("could not find %s on this machine; name the folder that holds them", strings.Join(config.MissingExternalTools(family, ""), " and "))
}

// externalSearchDirs is the PATH of this process and of the user's login shell:
// the dashboard runs as a service, with a PATH that lacks what a terminal has.
func externalSearchDirs() []string {
	dirs := filepath.SplitList(os.Getenv("PATH"))
	shell := os.Getenv("SHELL")
	if shell == "" {
		return dirs
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, shell, "-lc", "printenv PATH").Output(); err == nil {
		dirs = append(dirs, filepath.SplitList(strings.TrimSpace(string(out)))...)
	}
	return dirs
}

// ExternalShell runs shellCmd with a server's own clients, on this machine.
func ExternalShell(ctx context.Context, svc *config.CustomService, shellCmd string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", append([]string{"-c", shellCmd, "sh"}, args...)...)
	cmd.Env = config.ExternalDBToolEnv(svc, config.ServiceFilesDir(svc.Name))
	return cmd
}

// externalServiceFor is the external server behind a service name, or nil.
func externalServiceFor(name string) *config.CustomService {
	if svc, err := config.LoadCustomService(name); err == nil && svc.External != nil {
		return svc
	}
	return nil
}

// externalProbeFn asks the server its version; a seam for tests. It runs against a
// throwaway client file, so a password that fails replaces nothing.
var externalProbeFn = func(svc *config.CustomService, password string) (string, error) {
	home, err := os.MkdirTemp("", "lerd-external-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(home)
	if err := writeExternalClient(svc, home, password); err != nil {
		return "", err
	}
	query := `$(command -v mariadb || command -v mysql) -N -e 'SELECT VERSION()'`
	if config.ExternalDBFamily(svc.Family) == "postgres" {
		query = `psql -tAc 'SHOW server_version'`
	}
	version, err := externalProbeRun(svc, home, query)
	if err != nil {
		return "", err
	}
	if config.ExternalDBFamily(svc.Family) == "mysql" {
		return version, mysqlDumpFits(svc.External.Tools, version)
	}
	// pg_dump refuses a server newer than itself, so this fails now, not at the
	// first snapshot.
	dump, err := externalProbeRun(svc, home, "pg_dump --version")
	if err != nil {
		return "", err
	}
	if pgDumpTooOld(dump, version) {
		return "", fmt.Errorf("%s is older than the server's %s; use a newer one", dump, version)
	}
	return version, nil
}

// mysqlDumpFits refuses mariadb-dump against MySQL: it connects, then fails on the
// first dump over flags only MySQL's own tool knows.
func mysqlDumpFits(dir, version string) error {
	if strings.Contains(version, "MariaDB") || len(config.MissingExternalTools("mysql", dir)) > 0 {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "mysqldump")); err != nil {
		return fmt.Errorf("the server is MySQL %s, which needs mysqldump, and %s only has mariadb-dump; name a folder with MySQL's clients", version, dir)
	}
	return nil
}

// externalProbeTimeout bounds a test: a host dropping packets would otherwise hang
// the form for as long as the client cares to wait.
var externalProbeTimeout = 15 * time.Second

func externalProbeRun(svc *config.CustomService, home, shellCmd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), externalProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd)
	cmd.WaitDelay = time.Second
	cmd.Env = config.ExternalDBToolEnv(svc, home)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("no answer after %s", externalProbeTimeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

var firstNumber = regexp.MustCompile(`\d+`)

// pgDumpTooOld compares the majors of `pg_dump --version` and server_version.
func pgDumpTooOld(dump, server string) bool {
	d, _ := strconv.Atoi(firstNumber.FindString(dump))
	s, _ := strconv.Atoi(firstNumber.FindString(server))
	return d < s
}

// ExternalResult is what a connection test found.
type ExternalResult struct {
	Version string `json:"version"`
	Tools   string `json:"tools"`
}

// TestExternalDB connects to the server with the clients it would use, changing nothing.
func TestExternalDB(req ExternalRequest) (ExternalResult, error) {
	svc, err := externalService(req)
	if err != nil {
		return ExternalResult{}, err
	}
	version, err := externalProbeFn(svc, req.Password)
	if err != nil {
		return ExternalResult{}, err
	}
	return ExternalResult{Version: version, Tools: svc.External.Tools}, nil
}

// ExternalDBReachable says whether the server answers on its port.
func ExternalDBReachable(e config.ExternalDB) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(e.LocalHost(), strconv.Itoa(e.Port)), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ExternalDBStatus is a server's status: nothing runs here to ask, so it is
// whether the server answers.
func ExternalDBStatus(e config.ExternalDB) string {
	if ExternalDBReachable(e) {
		return "active"
	}
	return "offline"
}

// ServiceStatus is a service's status for anything deciding whether to talk to it:
// its unit's, or for an external server, which has none, whether it answers.
func ServiceStatus(name string) string {
	if svc := externalServiceFor(name); svc != nil {
		return ExternalDBStatus(*svc.External)
	}
	status, _ := podman.UnitStatus("lerd-" + name)
	return status
}

// AddExternalDB saves a server as a service. It connects first, so a wrong host,
// password or tools folder leaves nothing behind.
func AddExternalDB(req ExternalRequest) (*config.CustomService, error) {
	if ServiceInstalled(req.Name) || config.CustomServiceExists(req.Name) {
		return nil, fmt.Errorf("a service named %s already exists", req.Name)
	}
	if _, err := config.LoadPreset(req.Name); err == nil {
		return nil, fmt.Errorf("%s is the name of a lerd service; pick another", req.Name)
	}
	found, err := TestExternalDB(req)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s:%d: %w", containerHost(req.Host), req.Port, err)
	}
	// mysql is how most people name either server, and MySQL's dump flags fail on
	// MariaDB, so the server's own answer decides.
	if req.Engine == "mysql" && strings.Contains(found.Version, "MariaDB") {
		req.Engine, req.Version = "mariadb", ""
	}
	svc, err := externalService(req)
	if err != nil {
		return nil, err
	}
	svc.External.Version = found.Version
	if err := config.ValidateCustomService(svc); err != nil {
		return nil, err
	}
	if err := writeExternalCredentials(svc, req.Password); err != nil {
		return nil, err
	}
	if err := config.SaveCustomService(svc); err != nil {
		return nil, fmt.Errorf("saving service config: %w", err)
	}
	// Adding it again is the explicit install that lifts an earlier remove.
	if err := config.SetServiceRemoved(svc.Name, false); err != nil {
		return nil, err
	}
	return svc, nil
}

// UpdateExternalDB changes a server's password, its tools folder or both, an
// empty value keeping what is there. It connects with the result first, so a typo
// leaves the working setup alone.
func UpdateExternalDB(name, password, tools string) error {
	svc, err := config.LoadCustomService(name)
	if err != nil {
		return err
	}
	if svc.External == nil {
		return fmt.Errorf("%s is not an external server", name)
	}
	if password == "" {
		saved, err := os.ReadFile(config.ExternalDBPasswordFile(name))
		if err != nil {
			return err
		}
		password = string(saved)
	}
	if strings.TrimSpace(tools) != "" {
		if svc.External.Tools, err = externalToolsDir(svc.Family, tools); err != nil {
			return err
		}
	}
	if _, err := externalProbeFn(svc, password); err != nil {
		return fmt.Errorf("could not connect to %s: %w", svc.External.Address(), err)
	}
	if err := writeExternalCredentials(svc, password); err != nil {
		return err
	}
	return config.SaveCustomService(svc)
}

func writeExternalCredentials(svc *config.CustomService, password string) error {
	dir := config.ServiceFilesDir(svc.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(config.ExternalDBPasswordFile(svc.Name), []byte(password), 0o600); err != nil {
		return err
	}
	return writeExternalClient(svc, dir, password)
}

func writeExternalClient(svc *config.CustomService, dir, password string) error {
	return os.WriteFile(filepath.Join(dir, config.ExternalDBClientName(svc.Family)), []byte(config.ExternalDBClientContent(svc.Family, *svc.External, password)), 0o600)
}
