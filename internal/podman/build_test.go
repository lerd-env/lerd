package podman

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildCustomExtBlock_Empty(t *testing.T) {
	if got := buildCustomExtBlock(nil, nil); got != "" {
		t.Errorf("expected empty block for no extensions, got:\n%s", got)
	}
}

func TestBuildCustomExtBlock_BuiltinDeps(t *testing.T) {
	// imap's PECL build needs Alpine packages that aren't in the base image
	// (otherwise "U8T_CANONICAL is missing"), and it asks interactive prompts.
	block := buildCustomExtBlock([]string{"imap"}, nil)
	if !strings.Contains(block, "apk add --no-cache imap-dev krb5-dev openssl-dev c-client && ") {
		t.Errorf("imap block must apk add its built-in deps before installing:\n%s", block)
	}
	if !strings.Contains(block, "yes '' | pecl install imap") {
		t.Errorf("imap block must feed default answers to PECL prompts:\n%s", block)
	}
	if !strings.Contains(block, "|| true; }") {
		t.Errorf("block must keep the `|| true` resilience guard:\n%s", block)
	}
}

func TestBuildCustomExtBlock_UserDepsUnionedWithBuiltin(t *testing.T) {
	userDeps := map[string][]string{
		"imap": {"krb5-dev", "extra-pkg"}, // krb5-dev is also built-in → deduped
		"ssh2": {"libssh2-dev"},
	}
	block := buildCustomExtBlock([]string{"imap", "ssh2", "redis"}, userDeps)
	// imap: built-in (imap-dev krb5-dev openssl-dev c-client) ∪ user (krb5-dev extra-pkg)
	if !strings.Contains(block, "apk add --no-cache imap-dev krb5-dev openssl-dev c-client extra-pkg && ") {
		t.Errorf("imap block must union built-in + user deps without duplicates:\n%s", block)
	}
	// ssh2: only user deps
	if !strings.Contains(block, "apk add --no-cache libssh2-dev && ") {
		t.Errorf("ssh2 block must apk add the user-supplied dep:\n%s", block)
	}
	// redis: no deps anywhere → no apk add
	for _, line := range strings.Split(block, "\n") {
		if strings.Contains(line, "pecl install redis") && strings.Contains(line, "apk add") {
			t.Errorf("redis block should not have an apk add line:\n%s", line)
		}
	}
}

func TestApkDepsForExt_Dedup(t *testing.T) {
	got := apkDepsForExt("imap", map[string][]string{"imap": {" krb5-dev ", "extra", "", "imap-dev"}})
	want := []string{"imap-dev", "krb5-dev", "openssl-dev", "c-client", "extra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("apkDepsForExt(imap) = %v, want %v", got, want)
	}
	if apkDepsForExt("redis", nil) != nil {
		t.Errorf("apkDepsForExt(redis, nil) should be nil")
	}
}

func TestParseApkDeps(t *testing.T) {
	good := map[string][]string{
		"":                         nil,
		"   ":                      nil,
		"libssh2-dev":              {"libssh2-dev"},
		"imap-dev krb5-dev":        {"imap-dev", "krb5-dev"},
		"a, b ,c":                  {"a", "b", "c"},
		"libmemcached-dev\tzlib1g": {"libmemcached-dev", "zlib1g"},
		"openssl-dev c-client":     {"openssl-dev", "c-client"},
	}
	for in, want := range good {
		got, err := ParseApkDeps(in)
		if err != nil {
			t.Errorf("ParseApkDeps(%q) unexpected error: %v", in, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseApkDeps(%q) = %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"foo; rm -rf /", "$(whoami)", "a&&b", "with space evil`"} {
		if _, err := ParseApkDeps(bad); err == nil {
			t.Errorf("ParseApkDeps(%q) should have errored", bad)
		}
	}
}

func TestPhpExtensionLoaded(t *testing.T) {
	out := "Core\ndate\nimap\nPDO\nZend OPcache\n"
	cases := map[string]bool{
		"imap":         true,
		"IMAP":         true,
		" imap ":       true,
		"date":         true,
		"Zend OPcache": true,
		"pdo":          true,
		"imagick":      false,
		"":             false,
	}
	for ext, want := range cases {
		if got := phpExtensionLoaded(out, ext); got != want {
			t.Errorf("phpExtensionLoaded(out, %q) = %v, want %v", ext, got, want)
		}
	}
}
