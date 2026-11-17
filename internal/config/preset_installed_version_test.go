package config

import "testing"

func versionedPreset() *Preset {
	p := &Preset{Versions: []PresetVersion{
		{Tag: "7", Image: "docker.io/library/mongo:7", Canonical: true},
		{Tag: "9", Image: "docker.io/library/mongo:9"},
	}}
	p.Name = "mongo"
	return p
}

func TestInstalledVersion(t *testing.T) {
	cases := []struct {
		name string
		svc  CustomService
		want string
	}{
		{"a recorded version is kept", CustomService{Name: "mongo-9", PresetVersion: "9", Image: "docker.io/library/mongo:9"}, "9"},
		{"a single-image install is the canonical version", CustomService{Name: "mongo", Image: "docker.io/library/mongo:7"}, "7"},
		{"a patch update stays on the canonical line", CustomService{Name: "mongo", Image: "docker.io/library/mongo:7.0.48"}, "7"},
		{"another line is not guessed", CustomService{Name: "mongo", Image: "docker.io/library/mongo:9"}, ""},
		{"a look-alike tag is not the same line", CustomService{Name: "mongo", Image: "docker.io/library/mongo:70"}, ""},
		{"another repository is not guessed", CustomService{Name: "mongo", Image: "docker.io/other/mongo:7"}, ""},
		{"an alternate never takes the canonical tag", CustomService{Name: "mongo-8", Image: "docker.io/library/mongo:7"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := versionedPreset().InstalledVersion(&c.svc); got != c.want {
				t.Errorf("InstalledVersion = %q, want %q", got, c.want)
			}
		})
	}
}

func TestInstalledVersion_withoutCanonicalNothingIsGuessed(t *testing.T) {
	p := versionedPreset()
	p.Versions[0].Canonical = false
	if got := p.InstalledVersion(&CustomService{Name: "mongo", Image: "docker.io/library/mongo:7"}); got != "" {
		t.Errorf("InstalledVersion = %q, want empty", got)
	}
}

func TestInstalledVersion_singleImagePresetHasNoVersion(t *testing.T) {
	p := &Preset{}
	p.Name = "mongo"
	if got := p.InstalledVersion(&CustomService{Name: "mongo", Image: "docker.io/library/mongo:7"}); got != "" {
		t.Errorf("InstalledVersion = %q, want empty", got)
	}
}
