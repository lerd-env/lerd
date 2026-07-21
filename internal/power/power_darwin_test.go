//go:build darwin

package power

import "testing"

func TestOnBatteryFromPmset(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		want bool
	}{
		{"plugged in", "Now drawing from 'AC Power'\n -InternalBattery-0 (id=1)\t100%; charged; 0:00 remaining present: true\n", false},
		{"on battery", "Now drawing from 'Battery Power'\n -InternalBattery-0 (id=1)\t87%; discharging; 4:41 remaining present: true\n", true},
		{"pmset unavailable", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := onBatteryFromPmset(tc.out); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLowPowerFromPmset(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		want bool
	}{
		{"legacy key engaged", "System-wide power settings:\n lowpowermode         1\n", true},
		{"legacy key off", "System-wide power settings:\n lowpowermode         0\n", false},
		{"current key engaged", "Currently in use:\n powermode            1\n womp                 1\n", true},
		{"current key off", "Currently in use:\n powermode            0\n womp                 1\n", false},
		{"high power mode is not low power", "Currently in use:\n powermode            2\n", false},
		{"key absent entirely", "Currently in use:\n hibernatemode        3\n", false},
		{"pmset unavailable", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lowPowerFromPmset(tc.out); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Low Power Mode can be switched on while plugged in, and when it is the user
// has asked for less background work regardless of the power source.
func TestDetectState_LowPowerOutranksSource(t *testing.T) {
	prev := pmset
	pmset = func(args ...string) string {
		if len(args) == 1 && args[0] == "-g" {
			return " lowpowermode         1\n"
		}
		return "Now drawing from 'AC Power'\n"
	}
	t.Cleanup(func() { pmset = prev })

	if got := detectState(); got != LowPower {
		t.Errorf("low power while on AC = %v, want low-power", got)
	}
}

func TestDetectState_BatteryWithoutLowPower(t *testing.T) {
	prev := pmset
	pmset = func(args ...string) string {
		if len(args) == 1 && args[0] == "-g" {
			return " lowpowermode         0\n"
		}
		return "Now drawing from 'Battery Power'\n"
	}
	t.Cleanup(func() { pmset = prev })

	if got := detectState(); got != Battery {
		t.Errorf("got %v, want battery", got)
	}
}
