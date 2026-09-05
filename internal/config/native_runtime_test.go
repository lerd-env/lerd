package config

import "testing"

// The runtime is install-wide, so a site reports native from the global
// setting rather than from anything stored on the site itself.
func TestIsNativeFollowsTheGlobalMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)

	s := &Site{Name: "shop"}
	if s.IsNative() {
		t.Error("an install with no runtime configured serves from containers")
	}
}

// A native install still cannot move a site the shared FPM container does not
// serve, so those keep reporting non-native whatever the mode is.
func TestNativeIsNotFrankenPHPOrCustom(t *testing.T) {
	for _, s := range []*Site{
		{Runtime: "frankenphp"},
		{Runtime: "fpm-custom"},
		{ContainerPort: 8080},
		{HostPort: 3000},
	} {
		if s.ServedNatively(PHPRuntimeNative) {
			t.Errorf("%+v must not be served natively", s)
		}
	}
}
