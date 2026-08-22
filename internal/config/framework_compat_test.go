package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestFrameworkParse_IgnoresUnknownKeys is the backward-compatibility contract
// for the framework store, the same one presets already carry. A definition
// reaches every installed binary within a day and there is no version gate, so
// a key added for a newer lerd lands in an older one's parser first. It has to
// be ignored rather than fail the parse: a framework that fails to load takes
// the site's workers, console commands and doctor checks with it.
func TestFrameworkParse_IgnoresUnknownKeys(t *testing.T) {
	var fw Framework
	if err := yaml.Unmarshal([]byte(`
name: laravel
label: Laravel
workers:
  queue:
    label: Queue Worker
    command: php artisan queue:work
    requires_service:
      name: redis
      when_env: QUEUE_CONNECTION=redis
    some_key_from_a_newer_lerd: whatever
    nested_future_block:
      with: values
future_top_level: 1
`), &fw); err != nil {
		t.Fatalf("an unknown key from a newer store must not fail the parse: %v", err)
	}
	if fw.Name != "laravel" {
		t.Errorf("Name = %q, want the known fields still populated", fw.Name)
	}
	w, ok := fw.Workers["queue"]
	if !ok {
		t.Fatal("queue worker dropped")
	}
	if w.Command != "php artisan queue:work" {
		t.Errorf("command = %q, want the known worker fields still populated", w.Command)
	}
}

// TestLoadFrameworkYAML_KeepsWorkersAroundUnknownKeys is the same contract at
// the loader rather than the type: a store file carrying a field this binary
// has never heard of still has to produce a usable definition.
func TestLoadFrameworkYAML_KeepsWorkersAroundUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "laravel@13.yaml")
	body := `name: laravel
label: Laravel
workers:
  queue:
    label: Queue Worker
    command: php artisan queue:work
    tune_command: php artisan queue:work --queue={queue}
    unknown_future_key: yes
  horizon:
    command: php artisan horizon
    conflicts_with:
      - queue
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write definition: %v", err)
	}
	fw := loadFrameworkYAML(path)
	if fw == nil {
		t.Fatal("definition with an unknown key failed to load")
	}
	if len(fw.Workers) != 2 {
		t.Errorf("workers = %v, want both still present", fw.Workers)
	}
	if fw.Workers["queue"].TuneCommand == "" {
		t.Error("tune_command dropped")
	}
}

// TestFrameworkWorker_MissingRequiresServiceIsNoRequirement pins the other
// direction: a new binary reading a definition published before requires_service
// existed must treat the worker as having no service prerequisite, not as one
// with an empty service name.
func TestFrameworkWorker_MissingRequiresServiceIsNoRequirement(t *testing.T) {
	var fw Framework
	if err := yaml.Unmarshal([]byte(`
name: laravel
workers:
  queue:
    command: php artisan queue:work
`), &fw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if fw.Workers["queue"].RequiresService != nil {
		t.Error("a definition that declares no requires_service must leave it nil")
	}
}
