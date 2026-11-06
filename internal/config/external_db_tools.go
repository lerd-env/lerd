package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// externalTools are the clients each family needs, any one name per entry being
// enough: MariaDB and MySQL each ship under their own. The last name is the one
// people know, so it is the one an error uses.
func externalTools(family string) [][]string {
	if ExternalDBFamily(family) == "postgres" {
		return [][]string{{"psql"}, {"pg_dump"}}
	}
	return [][]string{{"mariadb", "mysql"}, {"mariadb-dump", "mysqldump"}}
}

// MissingExternalTools names the clients dir lacks.
func MissingExternalTools(family, dir string) []string {
	var missing []string
	for _, names := range externalTools(family) {
		found := false
		for _, name := range names {
			found = found || isExecutable(filepath.Join(dir, name))
		}
		if !found {
			missing = append(missing, names[len(names)-1])
		}
	}
	return missing
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// LocateExternalTools is the first folder holding every client. lerd's own bin
// folder never counts: its shims run inside lerd's containers.
func LocateExternalTools(family string, dirs []string) string {
	for _, dir := range dirs {
		if dir != "" && dir != BinDir() && len(MissingExternalTools(family, dir)) == 0 {
			return dir
		}
	}
	return ""
}

// VersionLabel is the server's version as lerd's service list shows one.
func (e ExternalDB) VersionLabel() string {
	if m := serverMajorMinor.FindString(e.Version); m != "" {
		return "v" + m
	}
	return ""
}

var serverMajorMinor = regexp.MustCompile(`^\d+(\.\d+)?`)

// ExternalDBClientName is the file each family's clients read in their home.
func ExternalDBClientName(family string) string {
	if ExternalDBFamily(family) == "postgres" {
		return ".pgpass"
	}
	return ".my.cnf"
}

// ExternalDBToolEnv is the environment a server's clients run in. home holds its
// client file, and PATH is rebuilt rather than extended so lerd's shims of the
// same names can never be picked up instead.
func ExternalDBToolEnv(svc *CustomService, home string) []string {
	env := []string{}
	for _, kv := range os.Environ() {
		if len(kv) >= 5 && (kv[:5] == "HOME=" || kv[:5] == "PATH=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+home, "PATH="+svc.External.Tools+":/usr/bin:/bin")
	if ExternalDBFamily(svc.Family) == "postgres" {
		// PGDATABASE because psql otherwise opens a database named after the user,
		// which an outside server rarely has.
		e := svc.External
		env = append(env, "PGHOST="+e.LocalHost(), "PGPORT="+strconv.Itoa(e.Port), "PGUSER="+e.User, "PGDATABASE=postgres")
	}
	return env
}
