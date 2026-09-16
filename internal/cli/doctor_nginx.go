package cli

// nginxServiceFinding turns the two facts the doctor can observe about nginx
// into the line it reports. Installed and running are different questions, and
// only the second decides whether any site answers: asking the first alone
// reported a healthy machine while nginx was down and every site served
// nothing.
//
// A stopped nginx is deliberately not given an auto fix. Starting it means
// `lerd start`, which reconfigures the resolver under sudo, and the auto tier
// promises never to do that.
func nginxServiceFinding(installed, running bool) (detail, hint string, healthy bool) {
	switch {
	case !installed:
		return "not installed", "run: lerd install", false
	case !running:
		return "installed but not running, so no site is being served", "run: lerd start", false
	default:
		return "", "", true
	}
}
