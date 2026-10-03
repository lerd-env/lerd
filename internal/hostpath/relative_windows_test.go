//go:build windows

package hostpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelToVMTurnsRelativeWindowsPathsIntoPosixOnes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, "tests", "Unit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "Unit", "FooTest.php"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`.\artisan`:               "./artisan",
		`..\shared\run.php`:       "../shared/run.php",
		`tests\Unit\FooTest.php`:  "tests/Unit/FooTest.php",
		`App\Models\User`:         `App\Models\User`,
		`--filter=Tests\Unit\Foo`: `--filter=Tests\Unit\Foo`,
		`C:\Sites\app\artisan`:    `C:\Sites\app\artisan`,
		"artisan":                 "artisan",
		"./artisan":               "./artisan",
		"":                        "",
	}
	for in, want := range cases {
		if got := RelToVM(in); got != want {
			t.Errorf("RelToVM(%q) = %q, want %q", in, got, want)
		}
	}
}
