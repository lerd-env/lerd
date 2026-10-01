package config

import (
	"os"
	"path/filepath"
	"sort"
)

// A built-in service is installed only while its unit exists, and uninstall
// removes every unit. When the data is kept this list is what lets the next
// install bring back the services the user had, not only the ones a site lists.
func keptServicesFile() string {
	return filepath.Join(DataDir(), "kept-services.yaml")
}

// SetKeptServices records the built-in services installed at uninstall time.
func SetKeptServices(names []string) error {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return saveServiceNameSet(keptServicesFile(), m)
}

// KeptServices returns the recorded services, sorted.
func KeptServices() []string {
	m, _ := loadServiceNameSet(keptServicesFile())
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ClearKeptServices forgets the record once an install has restored it.
func ClearKeptServices() error {
	err := os.Remove(keptServicesFile())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
