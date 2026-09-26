package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestServiceIdleSuspended_RoundTrip(t *testing.T) {
	setDataDir(t)

	if got := IdleSuspendedServices(); len(got) != 0 {
		t.Fatalf("empty store listed %v", got)
	}
	for _, n := range []string{"redis", "mysql"} {
		if err := SetServiceIdleSuspended(n, true); err != nil {
			t.Fatalf("SetServiceIdleSuspended %s: %v", n, err)
		}
	}
	if !ServiceIsIdleSuspended("mysql") {
		t.Fatal("expected mysql idle-suspended after Set(true)")
	}
	if got := IdleSuspendedServices(); !reflect.DeepEqual(got, []string{"mysql", "redis"}) {
		t.Fatalf("IdleSuspendedServices = %v, want sorted [mysql redis]", got)
	}
	if err := SetServiceIdleSuspended("mysql", false); err != nil {
		t.Fatal(err)
	}
	if ServiceIsIdleSuspended("mysql") || ServiceIsPaused("mysql") {
		t.Fatal("clearing idle-suspended left mysql flagged")
	}
}

func TestSiteWaitsOnSleepingService(t *testing.T) {
	setDataDir(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DB_HOST=lerd-mysql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AddSite(Site{Name: "shop", Path: dir, Domains: []string{"shop.test"}}); err != nil {
		t.Fatal(err)
	}
	if SiteWaitsOnSleepingService("shop") {
		t.Fatal("nothing asleep yet")
	}
	_ = SetServiceIdleSuspended("mysql", true)
	if !SiteWaitsOnSleepingService("shop") {
		t.Fatal("shop uses the sleeping mysql")
	}
	_ = SetServiceIdleSuspended("redis", true)
	_ = SetServiceIdleSuspended("mysql", false)
	if SiteWaitsOnSleepingService("shop") {
		t.Fatal("shop does not use redis")
	}
}
