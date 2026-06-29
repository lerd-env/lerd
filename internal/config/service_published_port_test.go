package config

import "testing"

// TestServicePublishedPort reflects a shifted published port (set by `lerd
// service port` or the port-ownership guard) so host-facing surfaces — a
// host-proxy app's .env, a connection URL — target where lerd's container
// actually listens, not the engine default a coexisting host server may own.
func TestServicePublishedPort(t *testing.T) {
	setConfigDir(t)

	// No override → the "use default" sentinel 0.
	if got := ServicePublishedPort("postgres"); got != 0 {
		t.Errorf("ServicePublishedPort(postgres) with no override = %d, want 0", got)
	}

	// Guard auto-shifts lerd-postgres off a host-owned :5432 onto :5434.
	cfg, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal: %v", err)
	}
	if cfg.Services == nil {
		cfg.Services = map[string]ServiceConfig{}
	}
	cfg.Services["postgres"] = ServiceConfig{Enabled: true, Port: 5432, PublishedPort: 5434}
	if err := SaveGlobal(cfg); err != nil {
		t.Fatalf("SaveGlobal: %v", err)
	}
	if got := ServicePublishedPort("postgres"); got != 5434 {
		t.Errorf("ServicePublishedPort(postgres) after shift = %d, want 5434", got)
	}

	// Unknown service → 0 (never panics on a missing entry).
	if got := ServicePublishedPort("does-not-exist"); got != 0 {
		t.Errorf("ServicePublishedPort(unknown) = %d, want 0", got)
	}
}
