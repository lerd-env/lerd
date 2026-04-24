package podman

import "testing"

func TestServiceVersionLabel(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"docker.io/library/mysql:8.0", "v8.0"},
		{"docker.io/library/redis:7-alpine", "v7"},
		{"docker.io/postgis/postgis:16-3.5", "v16"},
		{"docker.io/postgis/postgis:16-3.5-alpine", "v16"},
		{"docker.io/getmeili/meilisearch:v1.7", "v1.7"},
		{"docker.io/axllent/mailpit:latest", "latest"},
		{"docker.io/rustfs/rustfs:latest", "latest"},
		{"docker.io/library/redis:main", "main"},
		{"nginx:alpine", "alpine"},
		{"nginx", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if got := ServiceVersionLabel(tt.image); got != tt.want {
				t.Errorf("ServiceVersionLabel(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}
