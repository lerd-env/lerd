package cli

import (
	"reflect"
	"testing"
)

func TestPhpScriptArgIndex(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"bare script", []string{"artisan", "migrate"}, 0},
		{"absolute script", []string{"/tmp/ide-phpinfo.php"}, 0},
		{"glued -d before script", []string{"-dmemory_limit=-1", "/tmp/x.php"}, 1},
		{"separated -d before script", []string{"-d", "memory_limit=-1", "script.php"}, 2},
		{"script then data file arg", []string{"check.php", "/tmp/input.xml"}, 0},
		{"-r runs no file", []string{"-r", "echo 1;"}, -1},
		{"-v runs no file", []string{"-v"}, -1},
		{"-f names the script", []string{"-f", "/tmp/x.php"}, 1},
		{"empty", nil, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := phpScriptArgIndex(tt.args); got != tt.want {
				t.Errorf("phpScriptArgIndex(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}

func TestSpxPassthroughEnv(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    []string
	}{
		{
			name:    "no SPX vars yields nothing",
			environ: []string{"HOME=/home/x", "PATH=/bin"},
			want:    nil,
		},
		{
			name:    "enabled with no report defaults to full",
			environ: []string{"PATH=/bin", "SPX_ENABLED=1"},
			want:    []string{"SPX_ENABLED=1", "SPX_REPORT=full"},
		},
		{
			name:    "explicit report is left untouched",
			environ: []string{"SPX_ENABLED=1", "SPX_REPORT=fp"},
			want:    []string{"SPX_ENABLED=1", "SPX_REPORT=fp"},
		},
		{
			name:    "disabled SPX never gets a default report",
			environ: []string{"SPX_ENABLED=0"},
			want:    []string{"SPX_ENABLED=0"},
		},
		{
			name:    "other SPX vars are forwarded as-is",
			environ: []string{"SPX_SAMPLING_PERIOD=100", "SPX_ENABLED=1"},
			want:    []string{"SPX_SAMPLING_PERIOD=100", "SPX_ENABLED=1", "SPX_REPORT=full"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spxPassthroughEnv(tt.environ)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("spxPassthroughEnv(%v) = %v, want %v", tt.environ, got, tt.want)
			}
		})
	}
}
