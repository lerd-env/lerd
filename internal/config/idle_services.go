package config

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

func idleSuspendedServicesFile() string {
	return filepath.Join(DataDir(), "idle-suspended-services.yaml")
}

// ServiceIsIdleSuspended reports whether idle-suspend stopped the service
// because nothing was using it. Unlike a paused service, it wakes on its own.
func ServiceIsIdleSuspended(name string) bool {
	return serviceSetContains(idleSuspendedServicesFile(), name)
}

// SetServiceIdleSuspended marks or clears the idle-suspended flag for a service.
func SetServiceIdleSuspended(name string, v bool) error {
	return serviceSetUpdate(idleSuspendedServicesFile(), name, v)
}

// IdleSuspendedServices lists the services idle-suspend is holding asleep,
// sorted. A missing or unreadable file reads as none.
func IdleSuspendedServices() []string {
	m, err := loadServiceNameSet(idleSuspendedServicesFile())
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func idleSleptAtFile() string {
	return filepath.Join(DataDir(), "idle-slept-at.yaml")
}

// SetServiceSleptAt records when idle-suspend last put a service to sleep, so
// a scheduled snapshot can tell whether it changed since the last one.
func SetServiceSleptAt(name string, at time.Time) error {
	m := loadSleptAt()
	m[name] = at.Unix()
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	path := idleSleptAtFile()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	guardRealWrite(path)
	return os.WriteFile(path, data, 0644)
}

// ServiceSleptAt returns when idle-suspend last put the service to sleep.
func ServiceSleptAt(name string) (time.Time, bool) {
	ts, ok := loadSleptAt()[name]
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(ts, 0), true
}

func loadSleptAt() map[string]int64 {
	m := map[string]int64{}
	if data, err := os.ReadFile(idleSleptAtFile()); err == nil {
		_ = yaml.Unmarshal(data, &m)
	}
	return m
}

// SiteWaitsOnSleepingService reports whether any service the site uses is held
// asleep by idle-suspend, so whatever writes its vhost keeps it on the waking
// one: the real vhost would send requests to an app whose database is down.
func SiteWaitsOnSleepingService(siteName string) bool {
	for _, svc := range IdleSuspendedServices() {
		for _, s := range SitesUsingService(svc) {
			if s.Name == siteName {
				return true
			}
		}
	}
	return false
}
