package sitedoctor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/serviceops"
)

// checkServiceWiring compares the services a project picks in its .lerd.yaml
// with the ones its env file actually points at. A project can pick mysql and
// run on SQLite, which is what a Drupal site installed through its own
// installer does, and every other env check sits behind a dotenv gate that such
// a project never passes, so the site reports healthy while its database is
// somewhere else entirely.
//
// The question needs no parsing and no framework knowledge: the store declares
// which services a framework wires into env, and a project points at one when
// its config names the lerd-<service> container, which is a text question every
// format answers the same way.
func checkServiceWiring(path, envFile string, fw *config.Framework) (Check, bool) {
	return checkServiceWiringOn(path, envFile, fw, siteReachesServicesOnLoopback(path))
}

// siteReachesServicesOnLoopback reports whether a site's PHP runs on the host,
// which is where its env points at loopback and a published port rather than at
// a container name.
func siteReachesServicesOnLoopback(path string) bool {
	site, err := config.FindSiteByPath(path)
	if err != nil || site == nil {
		return false
	}
	return site.IsNative() || site.IsHostProxy()
}

func checkServiceWiringOn(path, envFile string, fw *config.Framework, loopback bool) (Check, bool) {
	if fw == nil || len(fw.Env.Services) == 0 {
		return Check{}, false
	}
	proj, err := config.LoadProjectConfig(path)
	if err != nil || proj == nil {
		return Check{}, false
	}
	picked := pickedServices(proj)
	if len(picked) == 0 {
		return Check{}, false
	}
	content, err := os.ReadFile(filepath.Join(path, envFile))
	if err != nil {
		// The env file is missing, which is the env-present check's finding.
		return Check{}, false
	}

	_, external := envfile.ReadOverride(path)
	var unwired []string
	for _, name := range picked {
		if name == "sqlite" || external[strings.ToLower(name)] {
			continue
		}
		if !frameworkWiresService(fw, name) {
			continue
		}
		if envfile.ReferencesContainer(string(content), name) {
			continue
		}
		if loopback && envfile.ReferencesLoopback(string(content), hostPorts(name), config.ServiceDomain(name)) {
			continue
		}
		unwired = append(unwired, name)
	}
	if len(unwired) == 0 {
		return Check{Name: "service_wiring", Status: StatusOK, Detail: "every service the project picks is wired into " + envFile}, true
	}
	return Check{
		Name:   "service_wiring",
		Status: StatusWarn,
		Fix:    FixEnvSync,
		Detail: strings.Join(unwired, ", ") + " picked in .lerd.yaml, but nothing in " + envFile +
			" points at it — run `lerd env` to write the connection, which repoints the app at that service",
	}, true
}

// pickedServices returns the services a project declares, from its services
// list and its db block, in a stable order.
func pickedServices(proj *config.ProjectConfig) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, svc := range proj.Services {
		add(svc.Name)
	}
	add(proj.DB.Service)
	sort.Strings(out)
	return out
}

// frameworkWiresService reports whether the framework declares how to wire a
// service into its env file, directly or through the family or role a drop-in
// stands in for, so a project on mariadb is measured against the mysql block
// that would have wired it.
func frameworkWiresService(fw *config.Framework, name string) bool {
	if _, ok := fw.Env.Services[name]; ok {
		return true
	}
	family := config.FamilyOfName(name)
	role := ""
	if svc, err := config.LoadCustomService(name); err == nil {
		role = config.EnvRoleOf(svc)
	}
	for declared := range fw.Env.Services {
		if declared == role {
			return true
		}
		if family != "" && config.FamilyOfName(declared) == family {
			return true
		}
	}
	return false
}

// hostPorts is the published host ports of a service, as strings, so an env
// value carrying one can be recognised. A mapping is "3411:3306", or
// "127.0.0.1:3307:3306" when it names an address, so the host port is always
// the field before the container port. A seam: tests name their own ports
// rather than depending on an installed service store.
var hostPorts = func(name string) []string {
	var ports []string
	for _, mapping := range serviceops.HostPortMappings(name) {
		parts := strings.Split(strings.TrimSuffix(mapping, "/tcp"), ":")
		if len(parts) >= 2 {
			ports = append(ports, parts[len(parts)-2])
		}
	}
	return ports
}
