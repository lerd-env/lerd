package envfile

import (
	"maps"
	"testing"
)

func TestCarriedValues(t *testing.T) {
	before := map[string]string{"REDIS_HOST": "lerd-redis", "DB_DATABASE": "acme", "APP_URL": "http://acme.test", "MAIL_PORT": "1025"}
	after := map[string]string{"REDIS_HOST": "127.0.0.1", "DB_DATABASE": "acme_shared", "APP_URL": "https://acme.test", "MAIL_PORT": "1025"}
	worktree := map[string]string{"REDIS_HOST": "lerd-redis", "DB_DATABASE": "acme_feat_x", "APP_URL": "http://feat-x.acme.test", "MAIL_PORT": "1025"}

	got := CarriedValues(worktree, before, after)
	want := map[string]string{"REDIS_HOST": "127.0.0.1"}
	if !maps.Equal(got, want) {
		t.Errorf("CarriedValues = %v, want %v", got, want)
	}
}
