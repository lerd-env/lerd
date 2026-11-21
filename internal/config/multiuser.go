package config

// SetMultiUser turns multi-user mode on or off.
func SetMultiUser(on bool) error {
	return mutateGlobal(func(c *GlobalConfig) error {
		c.MultiUser = on
		return nil
	})
}
