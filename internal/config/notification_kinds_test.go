package config

import "testing"

func TestNativeKindEnabledDefaults(t *testing.T) {
	c := &GlobalConfig{}
	// Everything on by default except dump.
	for _, k := range NotifyKinds {
		want := k != "dump"
		if got := c.NativeKindEnabled(k); got != want {
			t.Errorf("default %q = %v, want %v", k, got, want)
		}
	}
}

func TestSetNativeKindOverridesDefault(t *testing.T) {
	c := &GlobalConfig{}
	c.SetNativeKind("dump", true)
	c.SetNativeKind("mail", false)
	if !c.NativeKindEnabled("dump") {
		t.Error("dump should be enabled after opt-in")
	}
	if c.NativeKindEnabled("mail") {
		t.Error("mail should be disabled after opt-out")
	}
	eff := c.EffectiveNativeKinds()
	if len(eff) != len(NotifyKinds) {
		t.Fatalf("EffectiveNativeKinds len=%d, want %d", len(eff), len(NotifyKinds))
	}
	if eff["dump"] != true || eff["mail"] != false || eff["op_done"] != true {
		t.Errorf("effective map wrong: %+v", eff)
	}
}
