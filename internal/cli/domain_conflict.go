package cli

import (
	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// warnFilteredDomains prints a single-line warning for each domain that
// linker.ResolveDomains had to drop because another site already owns it.
// Includes the conflicting site name when known so the user can react.
func warnFilteredDomains(removed []string) {
	if len(removed) == 0 {
		return
	}
	for _, d := range removed {
		if existing, err := config.IsDomainUsed(d); err == nil && existing != nil {
			feedback.Warn("domain %q already used by site %q — skipped", d, existing.Name)
			continue
		}
		feedback.Warn("domain %q is reserved — skipped", d)
	}
}
