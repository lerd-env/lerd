//go:build windows

package cli

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestAutostartCommandQuotesThePath(t *testing.T) {
	got := autostartCommand(`C:\Users\me\My Tools\lerd.exe`)
	want := `"C:\Users\me\My Tools\lerd.exe" start`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestAutostartRoundTrip(t *testing.T) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Skip("Run key not readable")
	}
	k.Close()
	if _, err := readRunValue(); err == nil {
		t.Skip("a real lerd autostart entry exists; not touching it")
	}
	installAutostart()
	t.Cleanup(func() { _ = removeAutostart() })
	v, err := readRunValue()
	if err != nil || v == "" {
		t.Fatalf("value not written: %q %v", v, err)
	}
	if err := removeAutostart(); err != nil {
		t.Fatal(err)
	}
	if _, err := readRunValue(); err == nil {
		t.Error("value still present after removeAutostart")
	}
}

func readRunValue() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValueName)
	return v, err
}
