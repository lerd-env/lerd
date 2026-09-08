package serviceops

import (
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func setServiceCORSConfig(t *testing.T, on, optOut bool) {
	t.Helper()
	cfg, err := config.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	sc := cfg.Services["rustfs"]
	sc.DomainCORS = on
	sc.DomainCORSOptOut = optOut
	cfg.Services["rustfs"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
}

// The preset is where a service says a page talks to it directly, so a user who
// has expressed no opinion gets the answer the service definition carries.
func TestServiceDomainCORS_FollowsThePresetByDefault(t *testing.T) {
	domainTestConfig(t)
	if !serviceDomainCORS("rustfs") {
		t.Error("rustfs declares domain_cors, so it should be on with no user choice")
	}
	// A service whose preset says nothing stays off rather than being made
	// readable from a page nobody asked to let in.
	if serviceDomainCORS("postgres") {
		t.Error("a preset that does not ask for CORS should not get it")
	}
}

// Silence is not a no: only a recorded opt-out is, which is what stops a preset
// default from being handed back to a user who turned it off.
func TestServiceDomainCORS_OptOutBeatsThePreset(t *testing.T) {
	domainTestConfig(t)
	setServiceCORSConfig(t, false, true)
	if serviceDomainCORS("rustfs") {
		t.Error("the recorded opt-out did not survive the preset default")
	}
}

func TestServiceDomainCORS_UserChoiceTurnsItOnForAPresetThatIsSilent(t *testing.T) {
	domainTestConfig(t)
	cfg, _ := config.LoadGlobal()
	sc := cfg.Services["postgres"]
	sc.DomainCORS = true
	cfg.Services["postgres"] = sc
	if err := config.SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if !serviceDomainCORS("postgres") {
		t.Error("an explicit choice was ignored")
	}
}

// The vhost is what carries the preflight answer, so the resolved value has to
// reach it rather than being read only where it is stored.
func TestApplyServiceDomain_PassesCORSToTheVhost(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)

	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	if len(rec.cors) == 0 {
		t.Fatal("no vhost was written")
	}
	if !rec.cors[len(rec.cors)-1] {
		t.Error("rustfs declares domain_cors but the vhost was written without it")
	}
}

func TestSetServiceDomainCORS_RerendersTheVhost(t *testing.T) {
	domainTestConfig(t)
	rec := stubDomainSeams(t)
	if _, err := SetServiceDomain("rustfs", "rustfs", 0); err != nil {
		t.Fatalf("SetServiceDomain: %v", err)
	}
	before := len(rec.cors)

	if err := SetServiceDomainCORS("rustfs", false); err != nil {
		t.Fatalf("SetServiceDomainCORS: %v", err)
	}
	if len(rec.cors) <= before {
		t.Fatal("turning CORS off did not re-render the vhost")
	}
	if rec.cors[len(rec.cors)-1] {
		t.Error("the vhost was re-rendered still answering preflights")
	}
	if serviceDomainCORS("rustfs") {
		t.Error("the choice was not persisted")
	}
}

// The preflight answer lives in the domain's vhost, so a service with no domain
// has nowhere to carry it. Saving the choice anyway would leave a setting that
// silently does nothing.
func TestSetServiceDomainCORS_RefusedWithoutADomain(t *testing.T) {
	domainTestConfig(t)
	stubDomainSeams(t)
	err := SetServiceDomainCORS("rustfs", true)
	if err == nil || !strings.Contains(err.Error(), "no domain") {
		t.Fatalf("expected a missing-domain error, got %v", err)
	}
}
