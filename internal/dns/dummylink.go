package dns

// lerd0 is the always-up dummy link that keeps .test resolving when every real
// network link is down. Only the Linux NetworkManager + systemd-resolved path
// provisions it (see setup.go); macOS resolves .test through /etc/resolver and
// needs no equivalent. The names live here rather than in the linux-only setup
// file because the diagnostic chain is built for every platform and reports on
// the link by name.
const (
	lerdDummyIface   = "lerd0"
	lerdLinkUnitName = "lerd-dns-link.service"
	lerdLinkUnit     = "/etc/systemd/system/lerd-dns-link.service"
)
