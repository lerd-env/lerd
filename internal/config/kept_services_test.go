package config

import (
	"reflect"
	"testing"
)

func TestKeptServices_RoundTrip(t *testing.T) {
	setDataDir(t)

	if got := KeptServices(); len(got) != 0 {
		t.Fatalf("expected no kept services on an empty store, got %v", got)
	}
	if err := SetKeptServices([]string{"redis", "mysql"}); err != nil {
		t.Fatalf("SetKeptServices: %v", err)
	}
	if got, want := KeptServices(), []string{"mysql", "redis"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("KeptServices() = %v, want %v", got, want)
	}
	if err := ClearKeptServices(); err != nil {
		t.Fatalf("ClearKeptServices: %v", err)
	}
	if got := KeptServices(); len(got) != 0 {
		t.Fatalf("expected no kept services after clearing, got %v", got)
	}
}
