package dns

import "fmt"

// routesThroughResolved is false: Windows routes .test through an NRPT rule,
// with no systemd-resolved interfaces or dummy link to check.
const routesThroughResolved = false

// findListenerCmd returns the PowerShell command the user can run to identify
// the process bound to a TCP port.
func findListenerCmd(port int) string {
	return fmt.Sprintf("Get-Process -Id (Get-NetTCPConnection -LocalPort %d -State Listen).OwningProcess", port)
}

// defaultResolverHookup looks for the NRPT rule that sends .test to lerd-dns.
func defaultResolverHookup() (string, bool, string) {
	ns := nrptNamespace()
	return windowsNRPTKind, nrptRuleExists(ns), ns
}
