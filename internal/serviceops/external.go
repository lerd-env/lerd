package serviceops

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/podman"
)

// ExternalRequest is a database server lerd does not run. Engine is the preset
// whose client tools speak to it.
type ExternalRequest struct {
	Name, Engine, Version, Host string
	Port                        int
	User, Password              string
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

// externalService turns a preset into the client-only service for a server.
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
	ext := config.ExternalDB{Host: containerHost(req.Host), Port: req.Port, User: req.User}
	return &config.CustomService{
		Name:        req.Name,
		Image:       base.Image,
		Exec:        "sleep infinity",
		Init:        true,
		Family:      family,
		Label:       req.Name,
		Description: fmt.Sprintf("External %s at %s:%d", req.Engine, ext.Host, ext.Port),
		Category:    base.Category,
		Icon:        base.Icon,
		Color:       base.Color,
		Introspect:  introspect,
		Environment: config.ExternalDBEnv(family, ext),
		EnvVars:     base.EnvVars,
		External:    &ext,
	}, nil
}

// externalProbeFn asks the server its version from a throwaway client; a seam
// for tests. The credentials go in on stdin, so they never reach an argv.
var externalProbeFn = func(svc *config.CustomService, password string) (string, error) {
	target := config.ExternalDBClientTarget(svc.Family)
	query := `$(command -v mysql || command -v mariadb) -N -e 'SELECT VERSION()'`
	if config.ExternalDBFamily(svc.Family) == "postgres" {
		query = `psql -d postgres -tAc 'SHOW server_version'`
	}
	args := []string{"run", "-i", "--rm", "--network", "lerd", "--entrypoint", "sh"}
	for k, v := range svc.Environment {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, svc.Image, "-c", fmt.Sprintf("cat > %s; chmod 600 %s; exec %s", target, target, query))
	cmd := podman.Cmd(args...)
	cmd.Stdin = strings.NewReader(config.ExternalDBClientContent(svc.Family, *svc.External, password))
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// matchingVersion is the preset tag whose client suits a server, since a dump
// tool older than the server refuses to run: the same major, or the newest there is.
func matchingVersion(preset *config.Preset, serverVersion string) string {
	major := strings.SplitN(serverVersion, ".", 2)[0]
	newest, newestMajor := "", -1
	for _, v := range preset.Versions {
		tagMajor := strings.SplitN(v.Tag, ".", 2)[0]
		if tagMajor == major {
			return v.Tag
		}
		if n, err := strconv.Atoi(tagMajor); err == nil && n > newestMajor {
			newest, newestMajor = v.Tag, n
		}
	}
	return newest
}

// TestExternalDB connects to the server and returns its version, changing nothing.
func TestExternalDB(req ExternalRequest) (string, error) {
	svc, err := externalService(req)
	if err != nil {
		return "", err
	}
	if err := externalPullFn(svc.Image); err != nil {
		return "", err
	}
	return externalProbeFn(svc, req.Password)
}

// externalPullFn fetches a client image lerd does not have yet; a seam for tests.
var externalPullFn = func(image string) error {
	if podman.ImageExists(image) {
		return nil
	}
	return podman.PullImageWithProgress(image, func(string) {})
}

// ExternalDBReachable says whether the server answers on its port. It runs on the
// machine, where the container's name for it does not resolve.
func ExternalDBReachable(e config.ExternalDB) bool {
	host := e.Host
	if host == "host.containers.internal" {
		host = "127.0.0.1"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(e.Port)), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// externalStartFn brings the client up; a seam for tests.
var externalStartFn = func(svc *config.CustomService) error {
	if err := EnsureCustomServiceQuadlet(svc); err != nil {
		return err
	}
	return StartService(svc.Name)
}

// AddExternalDB saves a server as a service. It connects first, so a wrong host
// or password leaves nothing behind, and only then picks the client for the
// version it found.
func AddExternalDB(req ExternalRequest) (*config.CustomService, error) {
	if ServiceInstalled(req.Name) || config.CustomServiceExists(req.Name) {
		return nil, fmt.Errorf("a service named %s already exists", req.Name)
	}
	if _, err := config.LoadPreset(req.Name); err == nil {
		return nil, fmt.Errorf("%s is the name of a lerd service; pick another", req.Name)
	}
	version, err := TestExternalDB(req)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s:%d: %w", containerHost(req.Host), req.Port, err)
	}
	// mysql is how most people name either server, and MySQL's dump flags fail on
	// MariaDB, so the server's own answer decides.
	if req.Engine == "mysql" && strings.Contains(version, "MariaDB") {
		req.Engine, req.Version = "mariadb", ""
	}
	if req.Version == "" {
		if preset, err := config.EnsurePreset(req.Engine); err == nil {
			req.Version = matchingVersion(preset, version)
		}
	}
	svc, err := externalService(req)
	if err != nil {
		return nil, err
	}
	if err := config.ValidateCustomService(svc); err != nil {
		return nil, err
	}
	if err := externalPullFn(svc.Image); err != nil {
		return nil, err
	}
	if err := writeExternalClientFile(svc, req.Password); err != nil {
		return nil, err
	}
	if err := config.SaveCustomService(svc); err != nil {
		return nil, fmt.Errorf("saving service config: %w", err)
	}
	if err := externalStartFn(svc); err != nil {
		return svc, err
	}
	return svc, nil
}

// UpdateExternalDBPassword replaces a server's password, connecting with the new
// one first so a typo leaves the working one in place. The files are rewritten in
// place, so the running client sees the new password without a restart.
func UpdateExternalDBPassword(name, password string) error {
	svc, err := config.LoadCustomService(name)
	if err != nil {
		return err
	}
	if svc.External == nil {
		return fmt.Errorf("%s is not an external server", name)
	}
	if _, err := externalProbeFn(svc, password); err != nil {
		return fmt.Errorf("could not connect to %s: %w", svc.External.Address(), err)
	}
	return writeExternalClientFile(svc, password)
}

func writeExternalClientFile(svc *config.CustomService, password string) error {
	path := config.ExternalDBClientFile(svc.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(config.ExternalDBPasswordFile(svc.Name), []byte(password), 0o600); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(config.ExternalDBClientContent(svc.Family, *svc.External, password)), 0o600)
}
