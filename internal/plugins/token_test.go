package plugins

import (
	"os"
	"strings"
	"testing"
)

func TestToken_StablePerPluginAndResolvable(t *testing.T) {
	isolate(t)
	writePlugin(t, Dir(), "a", "api_version: 1\nname: a\n")
	writePlugin(t, Dir(), "b", "api_version: 1\nname: b\n")

	ta, err := Token("a")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Token("a")
	tb, _ := Token("b")
	if ta == "" || ta != again || ta == tb {
		t.Fatalf("tokens must be stable per plugin and differ between plugins: %q %q %q", ta, again, tb)
	}
	if got := PluginForToken(ta); got != "a" {
		t.Errorf("PluginForToken = %q, want a", got)
	}
	if got := PluginForToken("nope"); got != "" {
		t.Errorf("an unknown token must resolve to nothing, got %q", got)
	}
	if got := PluginForToken(""); got != "" {
		t.Errorf("an empty token must resolve to nothing, got %q", got)
	}
}

func TestToken_SecretIsPrivate(t *testing.T) {
	isolate(t)
	if _, err := Token("a"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(secretPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("secret mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestCommand_HandsOverAPIAccess(t *testing.T) {
	isolate(t)
	dir := writePlugin(t, Dir(), "p", "api_version: 1\nname: p\n")
	p, err := ReadPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := p.Command("bin/run", SurfaceCLI, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Token("p")
	env := strings.Join(cmd.Env, "\n")
	if !strings.Contains(env, "LERD_API_TOKEN="+want) || !strings.Contains(env, "LERD_API_URL="+APIURL) {
		t.Errorf("plugin env lacks API access:\n%s", env)
	}
}
