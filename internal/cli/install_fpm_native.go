package cli

// fpmVersionsToSettle splits the PHP versions into the ones whose shared FPM
// container an install should bring up and the ones it should take down, from
// where PHP runs.
//
// The native runtime serves every site from host pools, so these containers
// have nothing behind them. Declining to start them is not enough on its own:
// their quadlet restarts always, so an install that predates this left them
// running and podman keeps reviving them, which quietly undoes the teardown the
// runtime switch performed. They are stopped here instead.
func fpmVersionsToSettle(versions []string, wanted bool) (start, stop []string) {
	if len(versions) == 0 {
		return nil, nil
	}
	if wanted {
		return versions, nil
	}
	return nil, versions
}
