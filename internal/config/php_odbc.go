package config

import (
	"slices"
	"strings"
)

// ODBCDriver is a vendor ODBC driver registered with lerd (lerd php:odbc). The
// driver file stays where the vendor's installer put it on the host; lerd only
// names it in the odbcinst.ini every PHP container mounts, so a DSN's
// Driver={...} resolves to something the driver manager can load.
type ODBCDriver struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Driver      string `yaml:"driver" mapstructure:"driver"`
	Description string `yaml:"description,omitempty" mapstructure:"description"`
}

// GetODBCDrivers returns the registered drivers in registration order.
func (c *GlobalConfig) GetODBCDrivers() []ODBCDriver {
	return c.PHP.ODBCDrivers
}

// FindODBCDriver returns the driver registered under name. unixODBC resolves a
// DSN's Driver={...} without regard to case, so the lookup does too.
func (c *GlobalConfig) FindODBCDriver(name string) (ODBCDriver, bool) {
	for _, d := range c.PHP.ODBCDrivers {
		if strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return ODBCDriver{}, false
}

// SetODBCDriver registers d, replacing any driver already under that name so
// re-registering moves the path instead of listing the name twice.
func (c *GlobalConfig) SetODBCDriver(d ODBCDriver) {
	for i, existing := range c.PHP.ODBCDrivers {
		if strings.EqualFold(existing.Name, d.Name) {
			c.PHP.ODBCDrivers[i] = d
			return
		}
	}
	c.PHP.ODBCDrivers = append(c.PHP.ODBCDrivers, d)
}

// RemoveODBCDriver drops the driver registered under name, reporting whether
// there was one to drop.
func (c *GlobalConfig) RemoveODBCDriver(name string) bool {
	before := len(c.PHP.ODBCDrivers)
	c.PHP.ODBCDrivers = slices.DeleteFunc(c.PHP.ODBCDrivers, func(d ODBCDriver) bool {
		return strings.EqualFold(d.Name, name)
	})
	if len(c.PHP.ODBCDrivers) == 0 {
		c.PHP.ODBCDrivers = nil
	}
	return len(c.PHP.ODBCDrivers) != before
}
