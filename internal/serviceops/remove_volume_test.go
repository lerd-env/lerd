package serviceops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubVolumeOps(t *testing.T, exists bool) (exported, removed *[]string) {
	t.Helper()
	oe, or, ox := volumeExportFn, volumeRemoveFn, volumeExistsFn
	t.Cleanup(func() { volumeExportFn, volumeRemoveFn, volumeExistsFn = oe, or, ox })
	exported, removed = new([]string), new([]string)
	volumeExistsFn = func(string) bool { return exists }
	volumeExportFn = func(vol, path string) error {
		*exported = append(*exported, vol+"->"+filepath.Base(path))
		return os.WriteFile(path, []byte("tar"), 0644)
	}
	volumeRemoveFn = func(vol string) error { *removed = append(*removed, vol); return nil }
	return exported, removed
}

func TestRemoveServiceDataExportsTheVolumeBeforeDeletingIt(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	exported, removed := stubVolumeOps(t, true)

	if err := removeServiceData("mysql", true); err != nil {
		t.Fatal(err)
	}
	if len(*exported) != 1 || !strings.HasPrefix((*exported)[0], "lerd-data-mysql->mysql.pre-remove-") || !strings.HasSuffix((*exported)[0], ".tar") {
		t.Errorf("exported = %v, want one lerd-data-mysql tar", *exported)
	}
	if len(*removed) != 1 || (*removed)[0] != "lerd-data-mysql" {
		t.Errorf("removed = %v", *removed)
	}
}

func TestRemoveServiceDataKeepsTheVolumeWhenTheExportFails(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	_, removed := stubVolumeOps(t, true)
	volumeExportFn = func(string, string) error { return errors.New("disk full") }

	if err := removeServiceData("mysql", true); err == nil {
		t.Fatal("a failed export must fail the removal")
	}
	if len(*removed) != 0 {
		t.Errorf("the volume was deleted without a recoverable copy: %v", *removed)
	}
}

func TestRemoveServiceDataIsANoOpForAVolumeThatNeverExisted(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	exported, removed := stubVolumeOps(t, false)
	if err := removeServiceData("redis", true); err != nil {
		t.Fatal(err)
	}
	if len(*exported)+len(*removed) != 0 {
		t.Errorf("nothing should happen: exported=%v removed=%v", *exported, *removed)
	}
}

func TestRemoveServiceDataRenamesAHostDirAside(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	dir := filepath.Join(data, "lerd", "data", "pg")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := removeServiceData("pg", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the data dir should have been moved aside")
	}
	matches, _ := filepath.Glob(dir + ".pre-remove-*")
	if len(matches) != 1 {
		t.Errorf("aside copies = %v", matches)
	}
}
