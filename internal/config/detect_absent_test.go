package config

import "testing"

// A framework whose scaffold leaves the engine key unset (CakePHP inherits its
// driver from app.php) matched no service at all, so lerd wrote no connection
// and the site came up pointed at localhost. A rule can now say it applies when
// the key that would name another engine is absent.
func TestDetectRuleMatchesAnAbsentKey(t *testing.T) {
	absent := []FrameworkServiceDetect{
		{Key: "Datasources.default.driver", ValuePrefix: `Cake\Database\Driver\Mysql`},
		{Key: "Datasources.default.driver", Absent: true},
	}
	prefixOnly := []FrameworkServiceDetect{
		{Key: "Datasources.default.driver", ValuePrefix: `Cake\Database\Driver\Postgres`},
	}

	fresh := map[string]string{"Datasources.default.host": "localhost"}
	if !DetectRulesMatch(absent, fresh) {
		t.Error("an unset driver must match the rule that claims the default engine")
	}
	if DetectRulesMatch(prefixOnly, fresh) {
		t.Error("an unset driver must not match another engine")
	}

	pg := map[string]string{"Datasources.default.driver": `Cake\Database\Driver\Postgres`}
	if DetectRulesMatch(absent, pg) {
		t.Error("a driver that names postgres must not fall into the default engine")
	}
	if !DetectRulesMatch(prefixOnly, pg) {
		t.Error("postgres must still match its own rule")
	}

	my := map[string]string{"Datasources.default.driver": `Cake\Database\Driver\Mysql`}
	if !DetectRulesMatch(absent, my) {
		t.Error("an explicit mysql driver must still match")
	}
}

// No rules at all still means the declaration applies, as before.
func TestDetectRulesMatchWithNoRules(t *testing.T) {
	if !DetectRulesMatch(nil, map[string]string{}) {
		t.Error("a declaration with no rules has nothing to rule it out")
	}
}
