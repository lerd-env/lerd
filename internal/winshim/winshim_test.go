package winshim

import (
	"net"
	"reflect"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestConfig_RoundTrip(t *testing.T) {
	in := Config{Distro: "Ubuntu-24.04", Lerd: "/home/george/.local/bin/lerd", TLD: "test", Shims: "/home/george/.local/share/lerd/bin", Mount: "/mnt"}
	got, err := ParseConfig(in.String())
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("got %+v, want %+v", got, in)
	}
}

func TestParseConfig_MissingKeyFails(t *testing.T) {
	if _, err := ParseConfig("distro=Ubuntu\n"); err == nil {
		t.Error("a config without the lerd path must be refused")
	}
}

func TestLinuxPath(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{`\\wsl.localhost\Ubuntu-24.04\home\george\projects\app`, "/home/george/projects/app", true},
		{`\\wsl$\Ubuntu-24.04\home\george`, "/home/george", true},
		{`\\WSL.LOCALHOST\ubuntu-24.04\home`, "/home", true},
		{`\\wsl.localhost\Ubuntu-24.04`, "/", true},
		{`\\wsl.localhost\Debian\home`, "", false},
		{`C:\Users\george`, "", false},
	}
	for _, c := range cases {
		got, ok := LinuxPath(c.in, "Ubuntu-24.04")
		if got != c.want || ok != c.ok {
			t.Errorf("LinuxPath(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestWSLArgs(t *testing.T) {
	cfg := Config{Distro: "Ubuntu-24.04", Lerd: "/home/george/.local/bin/lerd", TLD: "test"}
	got := WSLArgs(cfg, `\\wsl.localhost\Ubuntu-24.04\home\george\app`, []string{"php", "-v"})
	want := []string{"-d", "Ubuntu-24.04", "--cd", "/home/george/app", "--exec", "/home/george/.local/bin/lerd", "php", "-v"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// A drive path goes to wsl.exe as-is; it translates C:\ to /mnt/c itself.
	got = WSLArgs(cfg, `C:\Users\george`, nil)
	if got[3] != `C:\Users\george` {
		t.Errorf("drive path rewritten: %q", got)
	}
}

// IDEs hand the shim the file they act on as a Windows path, which means nothing
// inside the distro; a bare word or a namespace with backslashes is left alone.
func TestArgToLinux(t *testing.T) {
	cfg := Config{Distro: "Ubuntu-24.04", Mount: "/mnt"}
	for in, want := range map[string]string{
		`C:\Users\me\app\artisan`:                        "/mnt/c/Users/me/app/artisan",
		`D:/work/x.php`:                                  "/mnt/d/work/x.php",
		`\\wsl.localhost\Ubuntu-24.04\home\me\app\a.php`: "/home/me/app/a.php",
		`\\wsl$\Ubuntu-24.04\home\me`:                    "/home/me",
		`--configuration=C:\app\phpunit.xml`:             "--configuration=/mnt/c/app/phpunit.xml",
		`--filter=App\\Tests\\FooTest`:                   `--filter=App\\Tests\\FooTest`,
		`App\Models\User`:                                `App\Models\User`,
		`-v`:                                             `-v`,
		`C:`:                                             `C:`,
		`\\wsl.localhost\Debian\home`:                    `\\wsl.localhost\Debian\home`,
	} {
		if got := ArgToLinux(cfg, in); got != want {
			t.Errorf("ArgToLinux(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShimArgs(t *testing.T) {
	cfg := Config{Distro: "Ubuntu-24.04", Lerd: "/home/me/.local/bin/lerd", Shims: "/home/me/.local/share/lerd/bin", Mount: "/mnt"}
	got, err := ShimArgs(cfg, `C:\app`, "composer", []string{"install", `C:\app\composer.json`})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-d", "Ubuntu-24.04", "--cd", `C:\app`, "--exec", "/home/me/.local/share/lerd/bin/composer", "install", "/mnt/c/app/composer.json"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	for _, bad := range []string{"../lerd", "a/b", "", "php;rm"} {
		if _, err := ShimArgs(cfg, `C:\app`, bad, nil); err == nil {
			t.Errorf("ShimArgs accepted shim name %q", bad)
		}
	}
	if _, err := ShimArgs(Config{Distro: "U", Lerd: "/l"}, `C:\app`, "php", nil); err == nil {
		t.Error("ShimArgs without a shims dir must fail and ask for wsl:setup")
	}
}

func query(t *testing.T, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 42, RecursionDesired: true})
	b.StartQuestions()
	if err := b.Question(dnsmessage.Question{Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET}); err != nil {
		t.Fatal(err)
	}
	out, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func parse(t *testing.T, resp []byte) dnsmessage.Message {
	t.Helper()
	var m dnsmessage.Message
	if err := m.Unpack(resp); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAnswer_A(t *testing.T) {
	resp, err := Answer(query(t, "app.branch.site.test.", dnsmessage.TypeA), "test")
	if err != nil {
		t.Fatal(err)
	}
	m := parse(t, resp)
	if m.ID != 42 || !m.Response || m.RCode != dnsmessage.RCodeSuccess || len(m.Answers) != 1 {
		t.Fatalf("bad response: %+v", m)
	}
	a := m.Answers[0].Body.(*dnsmessage.AResource)
	if !net.IP(a.A[:]).Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("got %v, want 127.0.0.1", net.IP(a.A[:]))
	}
}

func TestAnswer_AAAA(t *testing.T) {
	m := parse(t, mustAnswer(t, query(t, "Site.TEST.", dnsmessage.TypeAAAA), "test"))
	if len(m.Answers) != 1 {
		t.Fatalf("want one AAAA, got %+v", m.Answers)
	}
	a := m.Answers[0].Body.(*dnsmessage.AAAAResource)
	if !net.IP(a.AAAA[:]).Equal(net.IPv6loopback) {
		t.Errorf("got %v, want ::1", net.IP(a.AAAA[:]))
	}
}

func TestAnswer_OtherTypeIsEmptySuccess(t *testing.T) {
	m := parse(t, mustAnswer(t, query(t, "site.test.", dnsmessage.TypeMX), "test"))
	if m.RCode != dnsmessage.RCodeSuccess || len(m.Answers) != 0 {
		t.Errorf("want empty NOERROR, got %v with %d answers", m.RCode, len(m.Answers))
	}
}

func TestAnswer_OutsideTLDRefused(t *testing.T) {
	// Only the NRPT rule's namespace should ever arrive, but never answer for
	// names lerd does not own; "nottest." must not match on a bare suffix.
	for _, name := range []string{"example.com.", "nottest."} {
		m := parse(t, mustAnswer(t, query(t, name, dnsmessage.TypeA), "test"))
		if m.RCode != dnsmessage.RCodeRefused || len(m.Answers) != 0 {
			t.Errorf("%s: want REFUSED, got %v", name, m.RCode)
		}
	}
}

func mustAnswer(t *testing.T, q []byte, tld string) []byte {
	t.Helper()
	resp, err := Answer(q, tld)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestShimFromExe(t *testing.T) {
	for in, want := range map[string]string{
		`C:\x\lerd.exe`:       "",
		`C:\x\LERD.EXE`:       "",
		`C:\x\lerd-setup.exe`: "",
		`C:\x\php.exe`:        "php",
		`C:\x\composer.exe`:   "composer",
		`C:\x\redis-cli.exe`:  "redis-cli",
	} {
		if got := ShimFromExe(in); got != want {
			t.Errorf("ShimFromExe(%q) = %q, want %q", in, got, want)
		}
	}
}
