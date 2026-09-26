package config

import "path/filepath"

func removedServicesFile() string {
	return filepath.Join(DataDir(), "removed-services.yaml")
}

// ServiceIsRemoved reports whether the user removed the service. A site whose
// .lerd.yaml still lists it must not bring it back on its own; only installing
// it again explicitly does.
func ServiceIsRemoved(name string) bool {
	return serviceSetContains(removedServicesFile(), name)
}

// SetServiceRemoved marks or clears the removed flag for a service.
func SetServiceRemoved(name string, v bool) error {
	return serviceSetUpdate(removedServicesFile(), name, v)
}
