package config

import (
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
