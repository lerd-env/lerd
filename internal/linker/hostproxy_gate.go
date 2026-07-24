package linker

// HostProxyGate is the decision for whether lerd may supervise a dev-server
// command on the host. It is pure so the policy is testable without a terminal
// or a config file. proceed reports that the command may run without asking;
// ask reports that a confirmation is still owed; reason explains a refusal. A
// site only ever runs a command the user has consented to.
func HostProxyGate(command string, disabled, skipConfirm, approved, canPrompt bool) (proceed, ask bool, reason string) {
	if command == "" {
		// Proxy-only: lerd supervises nothing, so neither the disable switch
		// nor the confirmation applies.
		return true, false, ""
	}
	if disabled {
		return false, false, "host-proxy dev servers are disabled (set host_proxy.disabled: false to enable)"
	}
	if approved || skipConfirm {
		return true, false, ""
	}
	if !canPrompt {
		return false, false, "command not approved; re-run `lerd link` interactively, pass --yes, or set host_proxy.skip_confirmation: true"
	}
	return false, true, ""
}
