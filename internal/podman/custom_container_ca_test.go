package podman

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
)

func caFixture(t *testing.T, ca string) (string, *config.ContainerConfig) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CAROOT", root)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if ca != "" {
		if err := os.WriteFile(filepath.Join(root, "rootCA.pem"), []byte(ca), 0644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Containerfile.lerd"), []byte("FROM alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, &config.ContainerConfig{Port: 80, TrustCA: true, Target: "dev"}
}

// fakePodman stands in for podman: it writes the image ID the way --iidfile
// does, answers the inspect with user, and keeps what the CA layer was built from.
func fakePodman(t *testing.T, user string, failLayer bool) (calls *[][]string, layer *string) {
	t.Helper()
	calls, layer = &[][]string{}, new(string)
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	execCommand = func(_ string, args ...string) *exec.Cmd {
		*calls = append(*calls, args)
		for i, a := range args {
			switch a {
			case "--iidfile":
				return exec.Command("sh", "-c", `echo sha256:abc > "$1"`, "sh", args[i+1])
			case "inspect":
				return exec.Command("echo", user)
			case "-f":
				if body, err := os.ReadFile(args[i+1]); err == nil {
					*layer = string(body)
				}
			}
		}
		if failLayer {
			return exec.Command("false")
		}
		return exec.Command("true")
	}
	return calls, layer
}

func TestBuildCustomImage_TrustCALayersTheCA(t *testing.T) {
	dir, cfg := caFixture(t, "CERT")
	calls, layer := fakePodman(t, "node", false)
	if err := BuildCustomImageTo("mysite", dir, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 3 {
		t.Fatalf("calls = %v, want build, inspect, build", *calls)
	}
	base := strings.Join((*calls)[0], " ")
	if strings.Contains(base, "-t lerd-custom-mysite") || !strings.Contains(base, "--target dev") {
		t.Errorf("base build = %q, want it untagged and on its target", base)
	}
	if last := strings.Join((*calls)[2], " "); !strings.Contains(last, "-t lerd-custom-mysite:local") || strings.Contains(last, "--target") {
		t.Errorf("layer build = %q", last)
	}
	if !strings.HasPrefix(*layer, "FROM sha256:abc\nUSER 0\n") || !strings.HasSuffix(*layer, "USER node\n") {
		t.Errorf("layer must run as root and restore the user:\n%s", *layer)
	}
}

func TestBuildCustomImage_TrustCAWithoutAUserStaysRoot(t *testing.T) {
	dir, cfg := caFixture(t, "CERT")
	_, layer := fakePodman(t, "", false)
	if err := BuildCustomImageTo("mysite", dir, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Count(*layer, "USER") != 1 {
		t.Errorf("no user to restore:\n%s", *layer)
	}
}

func TestBuildCustomImage_TrustCAOffBuildsOnce(t *testing.T) {
	dir, cfg := caFixture(t, "CERT")
	cfg.TrustCA = false
	calls, _ := fakePodman(t, "", false)
	if err := BuildCustomImageTo("mysite", dir, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Errorf("calls = %v, want one build", *calls)
	}
}

func TestBuildCustomImage_TrustCAFailures(t *testing.T) {
	dir, cfg := caFixture(t, "")
	calls, _ := fakePodman(t, "", false)
	if err := BuildCustomImageTo("mysite", dir, cfg, io.Discard); err == nil || !strings.Contains(err.Error(), "lerd CA") {
		t.Errorf("missing CA: err = %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("no build should run without a CA, got %v", *calls)
	}

	dir, cfg = caFixture(t, "CERT")
	fakePodman(t, "", true)
	if err := BuildCustomImageTo("mysite", dir, cfg, io.Discard); err == nil || !strings.Contains(err.Error(), "trusting the lerd CA") {
		t.Errorf("failed layer: err = %v", err)
	}
}

func TestHashContainerfile_TrustCA(t *testing.T) {
	dir, cfg := caFixture(t, "CERT")
	on := hashContainerfile(dir, cfg)
	if err := os.WriteFile(filepath.Join(os.Getenv("CAROOT"), "rootCA.pem"), []byte("NEW"), 0644); err != nil {
		t.Fatal(err)
	}
	renewed := hashContainerfile(dir, cfg)
	if on == "" || renewed == on {
		t.Error("a CA made anew must change the hash")
	}
	cfg.TrustCA = false
	if off := hashContainerfile(dir, cfg); off == on || off == renewed {
		t.Error("turning trust_ca off must change the hash")
	}
}

// runTrustScript runs the script with every path it touches moved under root
// and only the tools an image would have, plus the named stubs, on PATH.
func runTrustScript(t *testing.T, root string, stubs map[string]string, env ...string) {
	t.Helper()
	bin := t.TempDir()
	for _, tool := range []string{"cat", "cp", "mkdir", "dirname"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeUnder(t, root, customCAPath, "LERD\n")
	script := strings.NewReplacer("/usr/local/", root+"/usr/local/", "/etc/", root+"/etc/").Replace(customCATrustScript)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = append([]string{"PATH=" + bin}, env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func writeUnder(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func readUnder(t *testing.T, root, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestTrustScript_DistroTools(t *testing.T) {
	root := t.TempDir()
	ran := filepath.Join(root, "ran")
	runTrustScript(t, root, map[string]string{
		"update-ca-certificates": "echo certs > " + ran,
		"update-ca-trust":        "echo trust > " + ran,
	})
	if got := readUnder(t, root, "ran"); got != "certs\n" {
		t.Errorf("Debian's tool must win, ran %q", got)
	}

	root = t.TempDir()
	runTrustScript(t, root, map[string]string{"update-ca-trust": "echo \"$@\" > " + filepath.Join(root, "ran")})
	if got := readUnder(t, root, "ran"); got != "extract\n" {
		t.Errorf("update-ca-trust got %q", got)
	}
	if got := readUnder(t, root, "/etc/pki/ca-trust/source/anchors/"+customCAFile); got != "LERD\n" {
		t.Errorf("anchor = %q", got)
	}
}

func TestTrustScript_AppendsToTheBundle(t *testing.T) {
	const debian, fedora, alpine = "/etc/ssl/certs/ca-certificates.crt", "/etc/pki/tls/certs/ca-bundle.crt", "/etc/ssl/cert.pem"
	for _, bundle := range []string{debian, fedora, alpine} {
		root := t.TempDir()
		writeUnder(t, root, bundle, "PUBLIC\n")
		runTrustScript(t, root, nil)
		if got := readUnder(t, root, bundle); got != "PUBLIC\n\nLERD\n" {
			t.Errorf("%s = %q, want the public roots kept", bundle, got)
		}
	}
	root := t.TempDir()
	runTrustScript(t, root, nil)
	if got := readUnder(t, root, debian); !strings.Contains(got, "LERD") {
		t.Errorf("no bundle at all: %q", got)
	}
}

func TestTrustScript_NodeKeepsTheCertificatesItHad(t *testing.T) {
	root := t.TempDir()
	writeUnder(t, root, "corp.pem", "CORP\n")
	runTrustScript(t, root, nil, "NODE_EXTRA_CA_CERTS="+filepath.Join(root, "corp.pem"))
	if got := readUnder(t, root, customNodeCA); got != "CORP\n\nLERD\n" {
		t.Errorf("node CA file = %q", got)
	}
	for _, env := range []string{"NODE_EXTRA_CA_CERTS=", "NODE_EXTRA_CA_CERTS=/nowhere.pem"} {
		root = t.TempDir()
		runTrustScript(t, root, nil, env)
		if got := readUnder(t, root, customNodeCA); got != "\nLERD\n" {
			t.Errorf("%s: node CA file = %q", env, got)
		}
	}
}
