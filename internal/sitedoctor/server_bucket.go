package sitedoctor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/serviceops"
)

// OwnedEntity is one entity a site owns on a service: a bucket today, whatever
// a preset declares an owner_env for tomorrow. Nothing here knows the key by
// name, so a project keeping its configuration in any format lerd can read is
// checked like any other.
type OwnedEntity struct {
	Service string
	Kind    string
	Label   string
	Name    string
}

// listEntityNames is the declaration-driven lookup, hooked so tests can drive
// the check without a running service.
var listEntityNames = func(service string, spec *config.EntitySpec) ([]string, error) {
	rows, err := serviceops.ListEntities(service, spec)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names, nil
}

// stubEntityLister swaps the lookup for a test and returns the restore func.
func stubEntityLister(fn func(string, *config.EntitySpec) ([]string, error)) func() {
	prev := listEntityNames
	listEntityNames = fn
	ForgetEntities("")
	return func() {
		listEntityNames = prev
		ForgetEntities("")
	}
}

// entityListTTL bounds how long one service's entity list is reused, for the
// same reason dbListTTL exists: `lerd doctor` sweeps every site and would
// otherwise pay an ephemeral container run per site to ask the same question.
const entityListTTL = 5 * time.Second

type entityListEntry struct {
	names []string
	err   error
	at    time.Time
}

var entityListCache = struct {
	sync.Mutex
	entries map[string]entityListEntry
}{entries: map[string]entityListEntry{}}

// cachedEntities is listEntityNames with the recent answer reused.
func cachedEntities(service string, spec *config.EntitySpec) ([]string, error) {
	key := service + "/" + spec.Kind
	entityListCache.Lock()
	defer entityListCache.Unlock()
	if e, ok := entityListCache.entries[key]; ok && time.Since(e.at) < entityListTTL {
		return e.names, e.err
	}
	names, err := listEntityNames(service, spec)
	entityListCache.entries[key] = entityListEntry{names: names, err: err, at: time.Now()}
	return names, err
}

// ForgetEntities drops one service's cached lists, or every service's when
// service is empty. The fix has to call it: the re-check that follows runs well
// inside the TTL and would otherwise read the list from before the create and
// report the bucket it just made as still missing.
func ForgetEntities(service string) {
	entityListCache.Lock()
	defer entityListCache.Unlock()
	if service == "" {
		entityListCache.entries = map[string]entityListEntry{}
		return
	}
	for key := range entityListCache.entries {
		if strings.HasPrefix(key, service+"/") {
			delete(entityListCache.entries, key)
		}
	}
}

// servicesDeclaringOwnedEntities returns the installed services whose preset
// declares an entity a site can own, defaults and custom services alike.
func servicesDeclaringOwnedEntities() []string {
	var out []string
	seen := map[string]bool{}
	consider := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		for _, spec := range serviceops.ServiceEntities(name) {
			if spec.OwnerEnv != "" {
				out = append(out, name)
				return
			}
		}
	}
	for _, name := range config.DefaultPresetNames() {
		consider(name)
	}
	if customs, err := config.ListCustomServices(); err == nil {
		for _, svc := range customs {
			consider(svc.Name)
		}
	}
	return out
}

// OwnedEntitiesFor returns the entities a project's env file claims on the
// services it points at. A value alone is not enough: the env has to reference
// the service too, so a project pointed at real AWS never claims a local bucket
// of the same name.
func OwnedEntitiesFor(path string) []OwnedEntity {
	file, format := config.EnvFileFor(path)
	vals := envfile.Values(filepath.Join(path, file), format)
	if len(vals) == 0 {
		return nil
	}
	var out []OwnedEntity
	for _, service := range servicesDeclaringOwnedEntities() {
		if !envReferencesService(vals, service) {
			continue
		}
		for _, spec := range serviceops.ServiceEntities(service) {
			if spec.OwnerEnv == "" {
				continue
			}
			name := strings.TrimSpace(vals[spec.OwnerEnv])
			if name == "" {
				continue
			}
			out = append(out, OwnedEntity{Service: service, Kind: spec.Kind, Label: entityLabel(spec), Name: name})
		}
	}
	return out
}

// envReferencesService reports whether any value in the env file points at the
// service's container, the same test that links an entity row to its site.
func envReferencesService(vals map[string]string, service string) bool {
	for _, v := range vals {
		if strings.Contains(v, "lerd-"+service) {
			return true
		}
	}
	return false
}

// entityLabel is the singular noun for one entity, taken from the declaration's
// plural label ("Buckets" -> "Bucket") so the finding reads in the service's own
// vocabulary rather than a word chosen here.
func entityLabel(spec config.EntitySpec) string {
	label := strings.TrimSpace(spec.Label)
	if label == "" {
		label = spec.Kind
	}
	return strings.TrimSuffix(label, "s")
}

// MissingOwnedEntities returns the entities a project claims that its service
// does not hold. Exported so the fix works the set out again rather than
// trusting the client, the same way the database fix does.
func MissingOwnedEntities(path string) []OwnedEntity {
	missing, _ := missingOwnedEntities(path)
	return missing
}

// missingOwnedEntities pairs the set with whether any service could be asked at
// all: an unreachable one leaves the site unjudged rather than reported as
// missing a bucket that may well be there.
func missingOwnedEntities(path string) (missing []OwnedEntity, checked bool) {
	for _, t := range OwnedEntitiesFor(path) {
		spec := serviceops.EntityFor(t.Service, t.Kind)
		if spec == nil {
			continue
		}
		names, err := cachedEntities(t.Service, spec)
		if err != nil {
			continue
		}
		checked = true
		found := false
		for _, n := range names {
			if n == t.Name {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, t)
		}
	}
	return missing, checked
}

// checkServerBucket fails when the bucket a site points at is not on the service
// holding it. Such a site serves every page fine and fails on the first upload,
// with nothing else in the doctor looking at object storage at all.
func checkServerBucket(path string) (Check, bool) {
	missing, checked := missingOwnedEntities(path)
	if !checked {
		// Either the project claims no entity on a lerd-run service, or none could
		// be asked. Neither is this check's to judge.
		return Check{}, false
	}
	if len(missing) == 0 {
		return Check{Name: "server_bucket", Status: StatusOK}, true
	}
	named := make([]string, 0, len(missing))
	for _, t := range missing {
		named = append(named, fmt.Sprintf("%q on %s", t.Name, t.Service))
	}
	label := missing[0].Label
	return Check{Name: "server_bucket", Status: StatusFail, Fix: FixCreateBucket,
		Detail: fmt.Sprintf("%s %s %s not exist. Create %s so uploads have somewhere to land.",
			Plural(len(missing), label, label+"s"), strings.Join(named, ", "),
			Plural(len(missing), "does", "do"), Plural(len(missing), "it", "them"))}, true
}
