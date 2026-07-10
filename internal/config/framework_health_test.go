package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// The vite worker's health block (mirrored in the lerd-env/frameworks store)
// must unmarshal into WorkerHealth so Detect can find where the dev server
// publishes its port.
func TestWorkerHealthUnmarshals(t *testing.T) {
	src := `
workers:
  vite:
    command: npm run dev
    host: true
    check:
      file: node_modules/vite
    health:
      url_file: public/hot
  queue:
    command: php artisan queue:work
`
	var fw Framework
	if err := yaml.Unmarshal([]byte(src), &fw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	vite, ok := fw.Workers["vite"]
	if !ok || vite.Health == nil {
		t.Fatalf("vite health block missing: %+v", fw.Workers["vite"])
	}
	if vite.Health.URLFile != "public/hot" {
		t.Errorf("url_file = %q, want public/hot", vite.Health.URLFile)
	}
	// A worker without the block leaves Health nil, so the probe is skipped.
	if q := fw.Workers["queue"]; q.Health != nil {
		t.Errorf("queue should have no health block, got %+v", q.Health)
	}
}
