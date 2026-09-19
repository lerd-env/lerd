package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/phpini"
	"github.com/geodro/lerd/internal/podman"
	"github.com/spf13/cobra"
)

// NewPhpOdbcCmd returns the php:odbc parent command, which registers vendor
// ODBC drivers in the odbcinst.ini every PHP container mounts. The images ship
// unixODBC and the odbc extensions; the licensed driver file stays on the host,
// so this names one rather than installing it.
func NewPhpOdbcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "php:odbc",
		Short: "Register ODBC drivers with the PHP containers",
	}
	cmd.AddCommand(newPhpOdbcAddCmd())
	cmd.AddCommand(newPhpOdbcRemoveCmd())
	cmd.AddCommand(newPhpOdbcListCmd())
	return cmd
}

func newPhpOdbcAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> <path-to-driver.so>",
		Short: "Register an ODBC driver under the name a DSN asks for",
		Long: "Registers a driver in the odbcinst.ini every PHP container mounts, so a DSN's\n" +
			"Driver={<name>} resolves to it. The driver file stays where it is on the host.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := nativeImageCommandRefusal("php:odbc"); err != nil {
				return err
			}
			name := strings.TrimSpace(args[0])
			if !podman.ValidODBCDriverName(name) {
				return fmt.Errorf("invalid driver name %q: letters, digits, spaces, dots, dashes and underscores only", args[0])
			}
			driver, err := odbcDriverPath(args[1])
			if err != nil {
				return err
			}
			desc, _ := cmd.Flags().GetString("description")

			entry := config.ODBCDriver{Name: name, Driver: driver, Description: strings.TrimSpace(desc)}
			if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
				c.SetODBCDriver(entry)
			}); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			feedback.Begin()
			feedback.Line("registering ODBC driver " + feedback.Val(name))
			feedback.Note(driver)
			if err := applyODBCChange(); err != nil {
				return err
			}

			// A driver the image cannot load is still registered: the entry is a
			// declaration, and unregistering it would lose the path. The closing
			// line has to stop short of promising a DSN that would fail, though.
			version, err := phpPkgVersion("")
			check := odbcCheckUnknown
			if err == nil {
				check = reportODBCDriverStatus(version, entry)
			}
			switch check {
			case odbcCheckFails:
				feedback.Done("driver " + feedback.Val(name) + " registered for every PHP version, but the PHP " + version + " image cannot load it yet")
			case odbcCheckUnknown:
				feedback.Done("driver " + feedback.Val(name) + " registered, but it was not read back from an image, so run 'lerd php:odbc list' once the runtime is up to see whether it loads")
			default:
				feedback.Done("driver " + feedback.Val(name) + " registered, use it as Driver={" + name + "} in a DSN")
			}
			return nil
		},
	}
	cmd.Flags().String("description", "", "description recorded alongside the driver")
	return cmd
}

func newPhpOdbcRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Unregister an ODBC driver",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := nativeImageCommandRefusal("php:odbc"); err != nil {
				return err
			}
			removed := false
			if err := config.UpdateGlobal(func(c *config.GlobalConfig) {
				removed = c.RemoveODBCDriver(args[0])
			}); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			if !removed {
				return fmt.Errorf("no ODBC driver registered as %q, run 'lerd php:odbc list' to see the registered ones", args[0])
			}
			feedback.Begin()
			feedback.Line("unregistering ODBC driver " + feedback.Val(args[0]))
			if err := applyODBCChange(); err != nil {
				return err
			}
			feedback.Done("driver " + feedback.Val(args[0]) + " unregistered")
			return nil
		},
	}
}

func newPhpOdbcListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the registered ODBC drivers and what the image makes of them",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := nativeImageCommandRefusal("php:odbc"); err != nil {
				return err
			}
			cfg, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			drivers := cfg.GetODBCDrivers()
			// The probe below bind-mounts the generated registry, and podman
			// refuses a run whose mount source is missing, which would read back
			// as every driver being invisible. Seed it before asking.
			if len(drivers) > 0 {
				if err := podman.EnsureOdbcInst(); err != nil {
					return fmt.Errorf("writing the ODBC driver registry: %w", err)
				}
			}
			if len(drivers) == 0 {
				fmt.Println("No ODBC drivers registered.")
				fmt.Println("The images ship unixODBC, ext-odbc and ext-pdo_odbc; register a vendor driver with:")
				fmt.Println("  lerd php:odbc add HDBODBC /path/to/libodbcHDB.so")
				return nil
			}
			version, verErr := phpPkgVersion("")
			fmt.Println("Registered drivers:")
			for _, d := range drivers {
				fmt.Printf("  %s\n    %s\n", d.Name, d.Driver)
				if d.Description != "" {
					fmt.Printf("    %s\n", d.Description)
				}
				if verErr != nil {
					fmt.Println("    not checked, no PHP version resolved here")
					continue
				}
				status, statusErr := podman.InspectODBCDriver(version, d)
				if statusErr != nil {
					fmt.Printf("    PHP %s: not checked, the image could not be read\n", version)
					continue
				}
				fmt.Printf("    PHP %s: %s\n", version, odbcStatusLine(status))
			}
			return nil
		},
	}
}

// odbcDriverPath validates a driver path: absolute (the container resolves it at
// the same path the host does), present, and free of the quote that would break
// out of the odbcinst entry and the in-container probe.
func odbcDriverPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if strings.ContainsAny(path, "'\n") {
		return "", fmt.Errorf("invalid driver path %q: quotes and newlines are not allowed", raw)
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolving driver path %q: %w", raw, err)
		}
		path = abs
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("driver %q not found on this machine: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("driver %q is a directory, point at the driver library itself", path)
	}
	final := odbcPathForContainer(path)
	// Refuse before anything is written. lerd mounts the driver's directory into
	// every PHP container, and a directory the container keeps its own runtime in
	// would be covered by the host's, leaving the unit restart-looping with every
	// site on that version down. Vendor packages do install drivers here, so the
	// way out has to be named.
	if dir := filepath.Dir(final); podman.ODBCDirShadowsRuntime(dir) {
		return "", fmt.Errorf("driver %q sits in %s, which the PHP container keeps its own libraries in: mounting it would take the container down. Copy the driver somewhere of its own (say ~/odbc-drivers) and register that path", final, dir)
	}
	return final, nil
}

// odbcPathForContainer rewrites a driver path into the spelling the PHP
// container will see. An ostree system (Silverblue, Kinoite) keeps home at
// /var/home behind a /home symlink, and the two distributions disagree about
// which one lands in passwd, so the quadlet's %h mount is /var/home/you on one
// and /home/you on the other. A driver under home therefore has to be named the
// way %h names it, or the registry points at a path that does not exist inside
// the container. Anything outside home is resolved instead, since lerd mounts it
// by that path itself.
func odbcPathForContainer(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return resolved
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return resolved
	}
	rel, err := filepath.Rel(resolvedHome, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return resolved
	}
	return filepath.Join(home, rel)
}

// applyODBCChange rewrites the registry and restarts every container that mounts
// it: the shared FPM containers with their per-site custom-FPM ones, and each
// FrankenPHP site, whose quadlet also carries the driver directories.
func applyODBCChange() error {
	if err := podman.EnsureOdbcInst(); err != nil {
		return fmt.Errorf("writing the ODBC driver registry: %w", err)
	}
	reg, regErr := config.LoadSites()
	// A custom-FPM site carries the mount in its own quadlet, and the restart
	// below does not rewrite one, so those are brought up to date first.
	if regErr == nil {
		for _, s := range reg.Sites {
			if s.Paused || s.Ignored || !s.IsCustomFPM() {
				continue
			}
			if err := podman.WriteCustomFPMQuadlet(s.Name, s.PHPVersion); err != nil {
				feedback.Warn("updating the quadlet for %s: %v", s.Name, err)
			}
		}
	}
	if err := phpini.Restart(phpini.SharedScope); err != nil {
		feedback.Warn("restarting PHP containers: %v", err)
	}
	if regErr != nil {
		return nil
	}
	for _, s := range reg.Sites {
		if s.Paused || s.Ignored || !s.IsFrankenPHP() {
			continue
		}
		if err := phpini.Restart("site:" + s.Name); err != nil {
			feedback.Warn("restarting %s: %v", s.Name, err)
		}
	}
	return nil
}

// reportODBCDriverStatus says what the image made of a freshly registered
// driver, and reports whether it would actually load. A driver that registers
// but cannot load is the case worth spelling out: unixODBC answers it as
// "file not found" later, whatever the real reason was.
func reportODBCDriverStatus(version string, d config.ODBCDriver) odbcCheck {
	status, err := podman.InspectODBCDriver(version, d)
	if err != nil {
		return odbcCheckUnknown
	}
	if !status.Found {
		feedback.Warn("PHP %s cannot see %s from inside the image; on macOS the Podman VM shares your home directory, so a driver outside it reaches no container", version, d.Driver)
		return odbcCheckFails
	}
	if len(status.Unresolved) > 0 {
		feedback.Warn("PHP %s cannot load the driver, it needs libraries the image does not have: %s", version, strings.Join(status.Unresolved, ", "))
		feedback.Note(odbcShimHint)
		return odbcCheckFails
	}
	if len(status.Symbols) > 0 {
		feedback.Warn("PHP %s cannot load the driver, gcompat does not carry these glibc symbols: %s", version, strings.Join(status.Symbols, ", "))
		feedback.Note(odbcShimHint)
		return odbcCheckFails
	}
	if !status.Registered {
		feedback.Warn("PHP %s does not list %s in odbcinst -q -d", version, d.Name)
		return odbcCheckFails
	}
	return odbcCheckLoads
}

// odbcCheck is what reading a driver back from the image established. The third
// case is the one that used to be folded into success: when the check cannot run
// at all, nothing has been established, and saying the driver is ready to use in
// a DSN is a claim nothing supports.
type odbcCheck int

const (
	odbcCheckLoads odbcCheck = iota
	odbcCheckFails
	odbcCheckUnknown
)

// odbcShimHint is what to do about a driver the loader cannot satisfy. The
// shim has to be built against the driver inside the image, which is a per-site
// Containerfile, so no amount of re-registering here will fix it.
const odbcShimHint = "a driver needing more than gcompat has to be patched in a per-site Containerfile, see the ODBC section of the PHP docs"

// odbcStatusLine renders an inspection as one line for php:odbc list.
func odbcStatusLine(s podman.ODBCDriverStatus) string {
	switch {
	case !s.Found:
		return "not visible to the container"
	case len(s.Unresolved) > 0:
		return "cannot load, missing libraries: " + strings.Join(s.Unresolved, ", ")
	case len(s.Symbols) > 0:
		return "cannot load, missing glibc symbols: " + strings.Join(s.Symbols, ", ")
	case !s.Registered:
		return "not listed by the driver manager"
	default:
		return "loads"
	}
}
