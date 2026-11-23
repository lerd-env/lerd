//go:build darwin

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/pfredirect"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/portforward"
)

// TestHasNonZeroExitCode covers the failure-detection helper for both
// container plists (mysql / postgres / …) and runtime-mode worker plists
// (queue / schedule / horizon — `/bin/sh worker.sh` → `podman exec`).
// The (never exited) sentinel must NOT count as failure: it's the brief
// in-flight window between Start() returning and ContainerCache catching
// up, and surfacing it as failed would trigger spurious heal cycles on
// every fresh start.
func TestHasNonZeroExitCode(t *testing.T) {
	cases := map[string]bool{
		"state = waiting\n\tlast exit code = 0\n":                  false,
		"state = waiting\n\tlast exit code = 1\n":                  true,
		"state = not running\n\tlast exit code = 1\n":              true,
		"state = not running\n\tlast exit code = 137\n":            true,
		"state = not running\n\tlast exit code = (never exited)\n": false,
		"state = running\n":                                        false, // no exit code line at all
		"":                                                         false,
	}
	for in, want := range cases {
		if got := hasNonZeroExitCode(in); got != want {
			t.Errorf("hasNonZeroExitCode(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestIsContainerPlist verifies the heuristic distinguishing container
// launchers from runtime-mode workers and native-binary plists. Misclassifying
// a runtime worker as container would cause the "container crashed
// post-detach" failure branch in UnitStatus to fire on idle workers.
func TestIsContainerPlist(t *testing.T) {
	containerOut := []byte(`	program = /opt/homebrew/bin/podman
	arguments = {
		/opt/homebrew/bin/podman
		run
		-d
		--restart=always
		--name
		lerd-mysql
	}`)
	if !isContainerPlist(containerOut) {
		t.Error("expected container plist to be classified as container")
	}

	runtimeOut := []byte(`	program = /bin/sh
	arguments = {
		/bin/sh
		/Users/x/.local/share/lerd/run/workers/lerd-queue-app.sh
	}`)
	if isContainerPlist(runtimeOut) {
		t.Error("runtime worker plist must NOT be classified as container")
	}

	binaryOut := []byte(`	program = /Users/x/.local/bin/lerd-tray
	arguments = { /Users/x/.local/bin/lerd-tray }`)
	if isContainerPlist(binaryOut) {
		t.Error("native-binary plist must NOT be classified as container")
	}
}

func TestXmlEscStr(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"a<b>c", "a&lt;b&gt;c"},
		{"a&b", "a&amp;b"},
		// Single and double quotes are valid in XML PCDATA — must NOT be
		// escaped as &#39;/&#34; because Apple's plist parser passes those
		// numeric character references through literally instead of decoding them.
		{`a"b`, `a"b`},
		{"a'b", "a'b"},
		{"X_FRAME_OPTIONS=''", "X_FRAME_OPTIONS=''"},
		{"", ""},
	}
	for _, tt := range tests {
		got := xmlEscStr(tt.in)
		if got != tt.want {
			t.Errorf("xmlEscStr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildPlist(t *testing.T) {
	plist := buildPlist("com.lerd.test", []string{"/bin/sh", "--flag"}, true, keepAliveAlways, "/tmp/out.log", "/tmp/err.log")

	checks := []string{
		`<string>com.lerd.test</string>`,
		`<string>/bin/sh</string>`,
		`<string>--flag</string>`,
		`<key>RunAtLoad</key>`,
		`<true/>`,
		`<key>KeepAlive</key>`,
		`<key>StandardOutPath</key>`,
		`<string>/tmp/out.log</string>`,
		`<key>StandardErrorPath</key>`,
		`<string>/tmp/err.log</string>`,
		`<?xml version="1.0"`,
		`<!DOCTYPE plist`,
	}
	for _, want := range checks {
		if !strings.Contains(plist, want) {
			t.Errorf("buildPlist output missing %q", want)
		}
	}
}

func TestBuildPlistNoOptionalFields(t *testing.T) {
	plist := buildPlist("com.lerd.minimal", []string{"/bin/true"}, false, keepAliveNever, "", "")

	if strings.Contains(plist, "RunAtLoad") {
		t.Error("expected no RunAtLoad key")
	}
	if strings.Contains(plist, "KeepAlive") {
		t.Error("expected no KeepAlive key")
	}
	if strings.Contains(plist, "StandardOutPath") {
		t.Error("expected no StandardOutPath key")
	}
}

// TestBuildPlistExitTimeoutOnlyForWatcher pins that the extended exit grace is
// scoped to the watcher, which runs the full teardown on logout. Granting it to
// every job would make launchd wait a minute on any unit that hangs on stop.
func TestBuildPlistExitTimeoutOnlyForWatcher(t *testing.T) {
	watcher := buildPlist(plistLabel(podman.WatcherUnit), []string{"/bin/true"}, true, keepAliveAlways, "", "")
	if !strings.Contains(watcher, "<key>ExitTimeOut</key>") {
		t.Error("watcher plist must set ExitTimeOut so the teardown survives launchd's default exit grace")
	}

	other := buildPlist(plistLabel("lerd-nginx"), []string{"/bin/true"}, true, keepAliveAlways, "", "")
	if strings.Contains(other, "ExitTimeOut") {
		t.Error("non-watcher plists must not set ExitTimeOut")
	}
}

// TestWatcherExitTimeoutFitsTheTeardown pins the grace to something the work can
// actually fit in. The containers stop first and a database asks for up to 60s
// to finish writing, so a grace that only covers them is spent before the
// machine stop begins, and launchd kills the watcher partway through the one
// step the teardown exists for.
func TestWatcherExitTimeoutFitsTheTeardown(t *testing.T) {
	const longestContainerStop = 60 // databases declare this in the service store
	const machineStopBudget = 90    // what lerd allows a machine stop elsewhere

	if watcherExitTimeout < longestContainerStop+machineStopBudget {
		t.Errorf("exit grace of %ds cannot fit a %ds container stop followed by a %ds machine stop",
			watcherExitTimeout, longestContainerStop, machineStopBudget)
	}

	plist := buildPlist(plistLabel(podman.WatcherUnit), []string{"/bin/true"}, true, keepAliveAlways, "", "")
	if !strings.Contains(plist, fmt.Sprintf("<integer>%d</integer>", watcherExitTimeout)) {
		t.Errorf("watcher plist does not carry the %ds grace:\n%s", watcherExitTimeout, plist)
	}
}

// TestBuildPlistAbandonProcessGroupForMachineOwners pins which jobs keep their
// process group alive on exit. vfkit and gvproxy reparent to init but stay in
// the group of whatever ran `podman machine start`, and launchd kills what is
// left in a job's group when the job exits, so any unit that can bring the VM
// up has to abandon its group or it takes the VM down with it.
func TestBuildPlistAbandonProcessGroupForMachineOwners(t *testing.T) {
	for _, unit := range []string{"lerd-autostart", "lerd-ui", "lerd-tray", podman.WatcherUnit} {
		plist := buildPlist(plistLabel(unit), []string{"/bin/true"}, true, keepAliveAlways, "", "")
		if !strings.Contains(plist, "<key>AbandonProcessGroup</key>") {
			t.Errorf("%s can start the Podman Machine, so it must abandon its process group on exit", unit)
		}
	}

	// Container and worker jobs never start the VM, and the group kill is how
	// their strays get cleaned up.
	for _, unit := range []string{"lerd-nginx", "lerd-horizon-app"} {
		plist := buildPlist(plistLabel(unit), []string{"/bin/true"}, false, keepAliveNever, "", "")
		if strings.Contains(plist, "AbandonProcessGroup") {
			t.Errorf("%s must keep launchd's process group cleanup", unit)
		}
	}
}

func TestBuildPlistOnFailureEmitsSuccessfulExitDict(t *testing.T) {
	plist := buildPlist("com.lerd.onfail", []string{"/bin/true"}, false, keepAliveOnFailure, "", "")

	if !strings.Contains(plist, "<key>KeepAlive</key>") {
		t.Fatal("expected KeepAlive key")
	}
	if !strings.Contains(plist, "<key>SuccessfulExit</key>") || !strings.Contains(plist, "<false/>") {
		t.Errorf("on-failure policy should emit KeepAlive: SuccessfulExit=false, got:\n%s", plist)
	}
	if strings.Contains(plist, "<key>KeepAlive</key>\n\t<true/>") {
		t.Errorf("on-failure must not emit bare KeepAlive=true, got:\n%s", plist)
	}
}

func TestBuildPlistXMLEscaping(t *testing.T) {
	plist := buildPlist("com.lerd.esc", []string{"/bin/echo", "a<b>&c"}, false, keepAliveNever, "", "")
	if !strings.Contains(plist, "a&lt;b&gt;&amp;c") {
		t.Error("arguments should be XML-escaped in plist")
	}
}

func TestParseSection(t *testing.T) {
	content := `[Service]
ExecStart=/bin/sh --flag
Type=simple

[Install]
WantedBy=default.target
`
	svc := parseSection(content, "Service")
	if got := svc["ExecStart"]; len(got) != 1 || got[0] != "/bin/sh --flag" {
		t.Errorf("ExecStart = %v, want [/bin/sh --flag]", got)
	}
	if got := svc["Type"]; len(got) != 1 || got[0] != "simple" {
		t.Errorf("Type = %v, want [simple]", got)
	}
	if got := svc["WantedBy"]; len(got) != 0 {
		t.Errorf("WantedBy should not be in [Service] section, got %v", got)
	}

	install := parseSection(content, "Install")
	if got := install["WantedBy"]; len(got) != 1 || got[0] != "default.target" {
		t.Errorf("WantedBy = %v, want [default.target]", got)
	}
}

func TestSplitSystemdExec(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"bare args", "frankenphp php-server -l :8000 -r public/",
			[]string{"frankenphp", "php-server", "-l", ":8000", "-r", "public/"}},
		{"quoted sh -c script kept whole",
			`sh -c "install-php-extensions pcntl >/dev/null && exec php artisan octane:start --workers=auto --watch --poll"`,
			[]string{"sh", "-c", "install-php-extensions pcntl >/dev/null && exec php artisan octane:start --workers=auto --watch --poll"}},
		{"escaped inner quote", `echo "say \"hi\" now"`,
			[]string{"echo", `say "hi" now`}},
		{"escaped backslash decoded", `cmd "a\\b"`, []string{"cmd", `a\b`}},
		{"trailing backslash before closing quote", `cmd "path\\"`, []string{"cmd", `path\`}},
		{"empty quoted arg preserved", `cmd ""`, []string{"cmd", ""}},
		{"collapses runs of whitespace", "a   b\tc", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSystemdExec(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d (%v), want %d (%v)", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("arg[%d] = %q, want %q (full: %v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

// TestContainerToPodmanArgsQuotedExec is the regression guard for FrankenPHP
// worker mode on macOS: the worker Exec= is a single double-quoted `sh -c`
// script, and it must survive translation as exactly three argv elements
// (sh, -c, <whole script>) rather than being word-split with literal quotes.
func TestContainerToPodmanArgsQuotedExec(t *testing.T) {
	content := `[Container]
Image=docker.io/dunglas/frankenphp:php8.5
ContainerName=lerd-fp-demo
Exec=sh -c "install-php-extensions pcntl >/dev/null && exec php artisan octane:start --server=frankenphp --host=0.0.0.0 --port=8000 --workers=auto --watch"
`
	args, err := containerToPodmanArgs(parseSection(content, "Container"))
	if err != nil {
		t.Fatal(err)
	}
	if len(args) < 3 {
		t.Fatalf("too few args: %v", args)
	}
	gotTail := args[len(args)-3:]
	wantTail := []string{"sh", "-c",
		"install-php-extensions pcntl >/dev/null && exec php artisan octane:start --server=frankenphp --host=0.0.0.0 --port=8000 --workers=auto --watch"}
	for i := range wantTail {
		if gotTail[i] != wantTail[i] {
			t.Fatalf("tail arg[%d] = %q, want %q (full: %v)", i, gotTail[i], wantTail[i], args)
		}
	}
	for _, a := range args {
		if strings.HasPrefix(a, `"`) || strings.HasSuffix(a, `"`) {
			t.Fatalf("argv retains a literal quote: %q (full: %v)", a, args)
		}
	}
}

// The ssh-agent quadlet clears its stale socket via `sh -c` before exec'ing the
// agent; that script must reach podman as one argv element on macOS too.
func TestSSHAgentQuadletExecSurvivesTranslation(t *testing.T) {
	args, err := containerToPodmanArgs(parseSection(podman.GenerateSSHAgentQuadlet("lerd-php85-fpm:local"), "Container"))
	if err != nil {
		t.Fatal(err)
	}
	gotTail := args[len(args)-3:]
	wantTail := []string{"sh", "-c",
		"rm -f " + podman.SSHAgentSocket + " && exec ssh-agent -D -a " + podman.SSHAgentSocket}
	for i := range wantTail {
		if gotTail[i] != wantTail[i] {
			t.Fatalf("tail arg[%d] = %q, want %q (full: %v)", i, gotTail[i], wantTail[i], args)
		}
	}
}

// Container units are started by reading the argv back out of the plist, so an
// arg holding XML-special characters (any `sh -c` script with && or a
// redirection) has to survive the escape/unescape round trip verbatim.
func TestPlistArgsRoundTripsXMLSpecials(t *testing.T) {
	script := "rm -f /ssh-agent/agent.sock && exec ssh-agent -D -a /ssh-agent/agent.sock 2>/dev/null"
	want := []string{"podman", "run", "sh", "-c", script}
	dir := t.TempDir()
	p := filepath.Join(dir, "unit.plist")
	if err := os.WriteFile(p, []byte(buildPlist("com.lerd.unit", want, false, keepAliveAlways, "", "")), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := plistArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("plistArgs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseSectionMultipleValues(t *testing.T) {
	content := `[Container]
Volume=/home:/home:z
Volume=/var:/var:ro
Image=test:local
`
	c := parseSection(content, "Container")
	if got := c["Volume"]; len(got) != 2 {
		t.Errorf("expected 2 Volume entries, got %d", len(got))
	}
}

func TestParseSectionSkipsComments(t *testing.T) {
	content := `[Service]
# This is a comment
; This is also a comment
ExecStart=/bin/true
`
	svc := parseSection(content, "Service")
	if len(svc) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(svc), svc)
	}
}

func TestStripSELinuxVolOpts(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/home:/home:z", "/home:/home"},
		{"/home:/home:Z", "/home:/home"},
		{"/home:/home:ro,z", "/home:/home:ro"},
		{"/home:/home:z,ro", "/home:/home:ro"},
		{"/home:/home:ro", "/home:/home:ro"},
		{"/home:/home", "/home:/home"},
		{"/home:/home:z,Z", "/home:/home"},
		{"/home:/home:ro,z,noexec", "/home:/home:ro,noexec"},
	}
	for _, tt := range tests {
		got := stripSELinuxVolOpts(tt.in)
		if got != tt.want {
			t.Errorf("stripSELinuxVolOpts(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// noLoopbackForward hides the host's real pf rule and launchd jobs.
func noLoopbackForward(t *testing.T) {
	t.Helper()
	prev, prevDir := pfredirect.AnchorPath, portforward.PlistDir
	pfredirect.AnchorPath, portforward.PlistDir = filepath.Join(t.TempDir(), "missing"), t.TempDir()
	t.Cleanup(func() { pfredirect.AnchorPath, portforward.PlistDir = prev, prevDir })
}

func TestContainerToPodmanArgs(t *testing.T) {
	noLoopbackForward(t)
	c := map[string][]string{
		"ContainerName": {"lerd-nginx"},
		"Image":         {"docker.io/library/nginx:latest"},
		"PublishPort":   {"127.0.0.1:80:80"},
		"Network":       {"lerd"},
		"Volume":        {"/home/user/sites:/home/user/sites:ro"},
		"Environment":   {"FOO=bar"},
	}
	args, err := containerToPodmanArgs(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	argStr := strings.Join(args, " ")
	for _, want := range []string{
		"run", "-d", "--restart=always",
		"--name", "lerd-nginx", "--replace",
		"--network", "lerd",
		"-p", "80:80",
		"-e", "FOO=bar",
		"docker.io/library/nginx:latest",
	} {
		if !strings.Contains(argStr, want) {
			t.Errorf("args missing %q in: %s", want, argStr)
		}
	}
}

// Quadlet's HostName= maps to --hostname on Linux via podman-systemd; the
// macOS path was dropping it, so `lerd shell` showed root@<container-id>.
func TestContainerToPodmanArgs_HostName(t *testing.T) {
	c := map[string][]string{
		"ContainerName": {"lerd-php84-fpm"},
		"Image":         {"localhost/lerd-php-fpm:8.4"},
		"HostName":      {"laptop"},
	}
	args, err := containerToPodmanArgs(c)
	if err != nil {
		t.Fatal(err)
	}
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, "--hostname laptop") {
		t.Errorf("HostName must produce --hostname: %s", argStr)
	}
}

func TestContainerToPodmanArgsNoImage(t *testing.T) {
	c := map[string][]string{
		"ContainerName": {"test"},
	}
	_, err := containerToPodmanArgs(c)
	if err == nil {
		t.Error("expected error for missing Image")
	}
}

func TestPlistLabel(t *testing.T) {
	if got := plistLabel("lerd-nginx"); got != "com.lerd.lerd-nginx" {
		t.Errorf("plistLabel = %q, want com.lerd.lerd-nginx", got)
	}
}

func TestParseServiceUnitRestartPolicy(t *testing.T) {
	cases := []struct {
		name string
		body string
		want keepAlivePolicy
	}{
		{"always", "[Service]\nExecStart=/bin/sh\nRestart=always\n", keepAliveAlways},
		{"on-failure", "[Service]\nExecStart=/bin/sh\nRestart=on-failure\n", keepAliveOnFailure},
		{"none", "[Service]\nExecStart=/bin/sh\n", keepAliveNever},
		{"no", "[Service]\nExecStart=/bin/sh\nRestart=no\n", keepAliveNever},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := parseServiceUnit("lerd-test", tc.body)
			if err != nil {
				t.Fatalf("parseServiceUnit: %v", err)
			}
			if got != tc.want {
				t.Errorf("Restart=%q → policy %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestWriteAndRemoveServiceUnit(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	// Ensure LaunchAgents + Logs dirs exist
	os.MkdirAll(filepath.Join(tmp, "Library", "LaunchAgents"), 0755)
	os.MkdirAll(filepath.Join(tmp, "Library", "Logs", "lerd"), 0755)

	mgr := &darwinServiceManager{}
	content := "[Service]\nExecStart=/bin/sh --flag\n"

	if err := mgr.WriteServiceUnit("lerd-watcher", content); err != nil {
		t.Fatalf("WriteServiceUnit: %v", err)
	}

	path := filepath.Join(tmp, "Library", "LaunchAgents", "lerd-watcher.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if !strings.Contains(string(data), "com.lerd.lerd-watcher") {
		t.Error("plist missing expected label")
	}
	if !strings.Contains(string(data), "/bin/sh") {
		t.Error("plist missing expected ExecStart binary")
	}

	// ListServiceUnits should find it
	units := mgr.ListServiceUnits("lerd-*")
	if len(units) != 1 || units[0] != "lerd-watcher" {
		t.Errorf("ListServiceUnits = %v, want [lerd-watcher]", units)
	}

	// Remove
	if err := mgr.RemoveServiceUnit("lerd-watcher"); err != nil {
		t.Fatalf("RemoveServiceUnit: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("plist should be removed")
	}
}

func TestWriteServiceUnitIfChangedNoChange(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, "Library", "LaunchAgents"), 0755)
	os.MkdirAll(filepath.Join(tmp, "Library", "Logs", "lerd"), 0755)

	mgr := &darwinServiceManager{}
	content := "[Service]\nExecStart=/bin/sh\n"

	mgr.WriteServiceUnit("lerd-test", content)

	changed, err := mgr.WriteServiceUnitIfChanged("lerd-test", content)
	if err != nil {
		t.Fatalf("WriteServiceUnitIfChanged: %v", err)
	}
	if changed {
		t.Error("expected no change on identical content")
	}
}

func TestWriteContainerUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, "Library", "LaunchAgents"), 0755)
	os.MkdirAll(filepath.Join(tmp, "Library", "Logs", "lerd"), 0755)

	mgr := &darwinServiceManager{}
	content := "[Container]\nContainerName=lerd-nginx\nImage=nginx:latest\nPublishPort=80:80\n"

	if err := mgr.WriteContainerUnit("lerd-nginx", content); err != nil {
		t.Fatalf("WriteContainerUnit: %v", err)
	}

	path := filepath.Join(tmp, "Library", "LaunchAgents", "lerd-nginx.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	plist := string(data)
	if !strings.Contains(plist, "podman") {
		t.Error("container plist should contain podman command")
	}
	if !strings.Contains(plist, "nginx:latest") {
		t.Error("container plist should contain image name")
	}

	if !mgr.ContainerUnitInstalled("lerd-nginx") {
		t.Error("ContainerUnitInstalled should return true")
	}
	if mgr.ContainerUnitInstalled("lerd-nonexistent") {
		t.Error("ContainerUnitInstalled should return false for missing unit")
	}
}

func TestIsEnabledBasedOnPlistExistence(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, "Library", "LaunchAgents"), 0755)

	mgr := &darwinServiceManager{}

	if mgr.IsEnabled("lerd-test") {
		t.Error("should not be enabled before plist exists")
	}

	os.WriteFile(
		filepath.Join(tmp, "Library", "LaunchAgents", "lerd-test.plist"),
		[]byte("placeholder"), 0644,
	)

	if !mgr.IsEnabled("lerd-test") {
		t.Error("should be enabled when plist exists")
	}
}

func TestUnquoteSystemdValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"FOO=bar"`, `FOO=bar`},
		{`"ES_JAVA_OPTS=-Xms512m -Xmx512m"`, `ES_JAVA_OPTS=-Xms512m -Xmx512m`},
		{`"http.cors.allow-origin=\"*\""`, `http.cors.allow-origin="*"`},
		{`FOO=bar`, `FOO=bar`},
		{`"unclosed`, `"unclosed`},
		{``, ``},
	}
	for _, tt := range cases {
		if got := unquoteSystemdValue(tt.in); got != tt.want {
			t.Errorf("unquoteSystemdValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestContainerToPodmanArgsQuotedEnv(t *testing.T) {
	c := map[string][]string{
		"ContainerName": {"lerd-elasticsearch"},
		"Image":         {"docker.elastic.co/elasticsearch/elasticsearch:8.13.4"},
		"Network":       {"lerd"},
		"Environment": {
			`"ES_JAVA_OPTS=-Xms512m -Xmx512m"`,
			`"http.cors.allow-origin=\"*\""`,
			`"discovery.type=single-node"`,
		},
		"UserNS": {"keep-id:uid=1000,gid=0"},
	}
	args, err := containerToPodmanArgs(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	argStr := strings.Join(args, " ")

	for _, want := range []string{
		"-e ES_JAVA_OPTS=-Xms512m -Xmx512m",
		`-e http.cors.allow-origin="*"`,
		"-e discovery.type=single-node",
		"--userns keep-id:uid=1000,gid=0",
	} {
		if !strings.Contains(argStr, want) {
			t.Errorf("args missing %q in:\n%s", want, argStr)
		}
	}
	// Confirm no literal quotes bleed through into env values
	for _, bad := range []string{`-e "ES_JAVA_OPTS`, `"discovery.type`} {
		if strings.Contains(argStr, bad) {
			t.Errorf("args contain literal quote in env: %q found in:\n%s", bad, argStr)
		}
	}
}

func TestStripPrivilegedIPBind_v4(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:80:80":     "80:80",
		"127.0.0.1:443:443":   "443:443",
		"127.0.0.1:5300:5300": "127.0.0.1:5300:5300",
		"0.0.0.0:80:80":       "80:80",
		"3306:3306":           "3306:3306",
		"127.0.0.1:80:80/tcp": "80:80/tcp",
		"192.168.1.1:8080:80": "192.168.1.1:8080:80",
	}
	for in, want := range cases {
		if got := stripPrivilegedIPBind(in); got != want {
			t.Errorf("stripPrivilegedIPBind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripPrivilegedIPBind_v6(t *testing.T) {
	cases := map[string]string{
		"[::1]:80:80":         "80:80",
		"[::1]:443:443":       "443:443",
		"[::1]:5300:5300":     "[::1]:5300:5300",
		"[::1]:5300:5300/udp": "[::1]:5300:5300/udp",
		"[::]:80:80":          "80:80",
		"[::1]:443:443/tcp":   "443:443/tcp",
	}
	for in, want := range cases {
		if got := stripPrivilegedIPBind(in); got != want {
			t.Errorf("stripPrivilegedIPBind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripIPv6PublishPorts(t *testing.T) {
	cases := map[string]string{
		// Loopback IPv6 dup is dropped; its IPv4 partner survives.
		"PublishPort=127.0.0.1:80:80\nPublishPort=[::1]:80:80": "PublishPort=127.0.0.1:80:80",
		// Bind-all IPv6 has no IPv4 partner: rewrite to 0.0.0.0 rather than drop.
		"PublishPort=[::]:80:80":   "PublishPort=0.0.0.0:80:80",
		"PublishPort=[::]:443:443": "PublishPort=0.0.0.0:443:443",
		// Plain IPv4 lines and non-PublishPort lines pass through untouched.
		"PublishPort=80:80": "PublishPort=80:80",
		"Network=lerd":      "Network=lerd",
		"[Container]":       "[Container]",
	}
	for in, want := range cases {
		if got := stripIPv6PublishPorts(in); got != want {
			t.Errorf("stripIPv6PublishPorts(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLANExposedPublishPortsSurvive guards the full macOS transform chain for a
// LAN-exposed container (lan.exposed: true). The regression: BindForLAN +
// PairIPv6Binds collapse "PublishPort=80:80" into the "[::]:" bind-all form, and
// stripIPv6PublishPorts used to drop every bracketed line — leaving lerd-nginx
// with no -p flags, so nothing reached the host. The published ports must survive
// as -p entries in the final podman run args.
func TestLANExposedPublishPortsSurvive(t *testing.T) {
	noLoopbackForward(t)
	content := "[Container]\n" +
		"Image=docker.io/library/nginx:alpine\n" +
		"Network=lerd\n" +
		"PublishPort=80:80\n" +
		"PublishPort=443:443\n"

	content = podman.BindForLAN(content, true) // lanExposed
	content = podman.PairIPv6Binds(content)
	content = stripIPv6PublishPorts(content)

	c := parseSection(content, "Container")
	args, err := containerToPodmanArgs(c)
	if err != nil {
		t.Fatalf("containerToPodmanArgs: %v", err)
	}

	joined := strings.Join(args, " ")
	for _, want := range []string{"-p 80:80", "-p 443:443"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in podman args, got: %s", want, joined)
		}
	}
}

// TestPrecreateBindMountDirs_SkipsNamedVolume confirms a bind mount's absolute
// source is pre-created while a named volume (bare name, e.g. the new
// lerd-ssh-agent:/ssh-agent) is left to podman, so MkdirAll never drops a stray
// relative directory into the process working directory.
func TestPrecreateBindMountDirs_SkipsNamedVolume(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	bind := filepath.Join(dir, "mnt", "data")
	precreateBindMountDirs([]string{
		"lerd-ssh-agent:/ssh-agent", // named volume: podman-managed, must be skipped
		bind + ":" + bind + ":rw",   // bind mount: absolute source, must be created
	})

	if _, err := os.Stat(filepath.Join(dir, "lerd-ssh-agent")); !os.IsNotExist(err) {
		t.Errorf("named volume created a stray relative dir lerd-ssh-agent (err=%v)", err)
	}
	if _, err := os.Stat(bind); err != nil {
		t.Errorf("bind-mount source not pre-created: %v", err)
	}
}

// TestBootout_MarksWatcherStops is the regression test for a logout the watcher
// invented. launchd delivers a bootout as the same SIGTERM a real logout does,
// so every bootout of the watcher has to mark itself first; otherwise `lerd
// start`, which boots the watcher out before re-bootstrapping it, makes the
// watcher tear down everything start had just brought up.
func TestBootout_MarksWatcherStops(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, ".local", "share"))

	// The marker is what this covers, not launchd, so the command is stubbed out
	// rather than aimed at a domain that cannot exist.
	prev := launchctl
	launchctl = func(...string) ([]byte, error) { return nil, nil }
	t.Cleanup(func() { launchctl = prev })

	bootout(podman.WatcherUnit, "gui/4294967290", "com.lerd.does-not-exist") //nolint:errcheck
	if !config.ConsumeWatcherManagedStop() {
		t.Error("booting out the watcher must mark the stop as lerd-initiated")
	}

	bootout("lerd-nginx", "gui/4294967290", "com.lerd.does-not-exist") //nolint:errcheck
	if config.ConsumeWatcherManagedStop() {
		t.Error("booting out any other unit must not mark a watcher stop")
	}
}

// TestQuadletStopTimeout_ReadsBothSpellings pins that macOS honours the declared
// window whichever way the generator wrote it. It emits StopTimeout= only when it
// could confirm podman is 5.0 or newer, and falls back to PodmanArgs= whenever the
// version probe fails to run, which is exactly what happens under launchd's
// restricted PATH. Reading only the first spelling would silently hold a database
// to the five second default on the platform this matters most on.
func TestQuadletStopTimeout_ReadsBothSpellings(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]string
		want int
	}{
		{"native key", map[string][]string{"StopTimeout": {"60"}}, 60},
		{"podmanargs fallback", map[string][]string{"PodmanArgs": {"--stop-timeout=60"}}, 60},
		{"fallback among other args", map[string][]string{"PodmanArgs": {"--init", "--stop-timeout=45"}}, 45},
		{"declared nowhere", map[string][]string{}, config.DefaultStopTimeout},
		{"unparseable", map[string][]string{"StopTimeout": {"soon"}}, config.DefaultStopTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quadletStopTimeout(tc.in); got != tc.want {
				t.Errorf("quadletStopTimeout = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestContainerToPodmanArgs_StopTimeoutAppearsOnce pins that the fallback
// spelling does not also reach podman through the PodmanArgs passthrough. Two
// copies of the flag would leave which one applies resting on argument order.
func TestContainerToPodmanArgs_StopTimeoutAppearsOnce(t *testing.T) {
	args, err := containerToPodmanArgs(map[string][]string{
		"ContainerName": {"lerd-postgres"},
		"Image":         {"docker.io/library/postgres:17"},
		"PodmanArgs":    {"--init --stop-timeout=60"},
	})
	if err != nil {
		t.Fatalf("containerToPodmanArgs: %v", err)
	}

	seen := 0
	for _, a := range args {
		if strings.HasPrefix(a, stopTimeoutFlag+"=") {
			seen++
			if a != stopTimeoutFlag+"=60" {
				t.Errorf("stop timeout = %q, want the declared 60", a)
			}
		}
	}
	if seen != 1 {
		t.Errorf("%s appeared %d times, want exactly once: %v", stopTimeoutFlag, seen, args)
	}
	if !slices.Contains(args, "--init") {
		t.Errorf("the other PodmanArgs must still pass through: %v", args)
	}
}

// A loopback bind the pf redirect covers moves to its high port, which gvproxy
// can hold on 127.0.0.1, instead of being widened to every interface. A wide
// bind keeps the high port too, or loopback 80 would redirect to nothing.
func TestPublishArgsMovesRedirectedLoopbackPorts(t *testing.T) {
	redirected := map[int]bool{80: true, 443: true}
	cases := map[string][]string{
		"127.0.0.1:80:80":       {"127.0.0.1:17080:80"},
		"127.0.0.1:443:443/tcp": {"127.0.0.1:17443:443/tcp"},
		"80:80":                 {"80:80", "127.0.0.1:17080:80"},
		"0.0.0.0:443:443":       {"443:443", "127.0.0.1:17443:443"},
		"127.0.0.1:25:25":       {"25:25"},
		"127.0.0.1:3306:3306":   {"127.0.0.1:3306:3306"},
	}
	for in, want := range cases {
		if got := publishArgs(in, redirected, false); !slices.Equal(got, want) {
			t.Errorf("publishArgs(%q) = %q, want %q", in, got, want)
		}
	}
	if got := publishArgs("127.0.0.1:80:80", nil, false); !slices.Equal(got, []string{"80:80"}) {
		t.Errorf("without a redirect the bind must stay reachable, got %q", got)
	}
}

// The launchd jobs hold 80 and 443 on every interface as root, so a wide
// publish there could never bind; the forward serves the LAN instead.
func TestPublishArgsLeavesTheLANToTheLaunchdForward(t *testing.T) {
	redirected := map[int]bool{80: true}
	if got := publishArgs("0.0.0.0:80:80", redirected, true); !slices.Equal(got, []string{"127.0.0.1:17080:80"}) {
		t.Errorf("publishArgs = %q, want only the loopback target", got)
	}
}
