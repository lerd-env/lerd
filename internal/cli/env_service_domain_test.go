package cli

import "testing"

// The endpoint an app signs against and the URL the browser opens have to be
// the same host, so both shapes lerd writes have to land on the domain.
func TestApplyServiceDomainEnv_RepointsBothShapesAtTheDomain(t *testing.T) {
	updates := map[string]string{
		"AWS_ENDPOINT": "http://lerd-rustfs:9000",
		"AWS_URL":      "http://localhost:9000/uploads",
		"REDIS_HOST":   "lerd-redis",
		"MAIL_HOST":    "lerd-mailpit",
	}
	applyServiceDomainEnv(updates,
		map[string]string{"rustfs": "rustfs.test"},
		map[string]int{"rustfs": 9000})

	if got := updates["AWS_ENDPOINT"]; got != "https://rustfs.test" {
		t.Errorf("AWS_ENDPOINT = %q, want https://rustfs.test", got)
	}
	if got := updates["AWS_URL"]; got != "https://rustfs.test/uploads" {
		t.Errorf("AWS_URL = %q, want https://rustfs.test/uploads", got)
	}
	// A bare connection host is not a URL a browser ever sees, and rewriting it
	// would point the app at nginx instead of the service.
	if updates["REDIS_HOST"] != "lerd-redis" || updates["MAIL_HOST"] != "lerd-mailpit" {
		t.Errorf("bare hosts were rewritten: redis=%q mail=%q", updates["REDIS_HOST"], updates["MAIL_HOST"])
	}
}

// nginx serves the domain on 443, so the container port carried over from the
// old value would point at a port nothing listens on.
func TestApplyServiceDomainEnv_DropsThePortTheSchemeImplies(t *testing.T) {
	updates := map[string]string{"AWS_ENDPOINT": "http://lerd-rustfs:9000/"}
	applyServiceDomainEnv(updates,
		map[string]string{"rustfs": "rustfs.test"},
		map[string]int{"rustfs": 9000})
	if got := updates["AWS_ENDPOINT"]; got != "https://rustfs.test/" {
		t.Errorf("AWS_ENDPOINT = %q, want https://rustfs.test/", got)
	}
}

func TestApplyServiceDomainEnv_NoDomainsLeavesEverythingAlone(t *testing.T) {
	updates := map[string]string{"AWS_ENDPOINT": "http://lerd-rustfs:9000"}
	applyServiceDomainEnv(updates, nil, nil)
	if updates["AWS_ENDPOINT"] != "http://lerd-rustfs:9000" {
		t.Errorf("value changed with no domains configured: %q", updates["AWS_ENDPOINT"])
	}
}

// A value naming a host that only shares the port must keep its own port.
func TestApplyServiceDomainEnv_LeavesOtherHostsPortsIntact(t *testing.T) {
	updates := map[string]string{"OTHER_URL": "http://example.test:9000/thing"}
	applyServiceDomainEnv(updates,
		map[string]string{"rustfs": "rustfs.test"},
		map[string]int{"rustfs": 9000})
	if got := updates["OTHER_URL"]; got != "http://example.test:9000/thing" {
		t.Errorf("OTHER_URL = %q, want it untouched", got)
	}
}
