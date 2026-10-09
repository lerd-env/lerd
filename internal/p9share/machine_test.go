package p9share

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const machineConfig = `{
 "Name": "podman-machine-default",
 "Mounts": [
  {"Source": "C:\\Users\\me", "Target": "/Users/me", "Type": "9p", "VSockNumber": 58218},
  {"Source": "C:\\", "Target": "/mnt/c", "Type": "9p", "VSockNumber": 58219},
  {"Source": "C:\\Users\\me\\AppData\\Roaming\\containers", "Target": "/etc/containers", "Type": "9p", "VSockNumber": 58220},
  {"Source": "C:\\x", "Target": "/x", "Type": "virtiofs", "VSockNumber": 0}
 ]
}`

func TestReadMachineMountsKeepsThe9pOnes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine.json")
	if err := os.WriteFile(path, []byte(machineConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadMachineMounts(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Mount{
		{Source: `C:\Users\me`, Target: "/Users/me", Port: 58218},
		{Source: `C:\`, Target: "/mnt/c", Port: 58219},
		{Source: `C:\Users\me\AppData\Roaming\containers`, Target: "/etc/containers", Port: 58220},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadMachineMounts = %+v", got)
	}
}

func TestRemountsPairsSharesWithTheirVMFolder(t *testing.T) {
	mounts := []Mount{{Source: `C:\`, Target: "/mnt/c", Port: 58219}, {Source: `C:\Users\me`, Target: "/Users/me", Port: 58218}}
	shares := []Share{{Dir: `C:\Users\me`, Service: "0000e36a-facb-11e6-bd58-64006a7986d3"}, {Dir: `C:\`, Service: "0000e36b-facb-11e6-bd58-64006a7986d3"}}
	got, err := Remounts(shares, mounts)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Mount{mounts[1], mounts[0]}; !reflect.DeepEqual(got, want) {
		t.Errorf("Remounts = %+v, want %+v", got, want)
	}
	if _, err := Remounts(append(shares, Share{Dir: `D:\`, Service: "0000e36c-facb-11e6-bd58-64006a7986d3"}), mounts); err == nil {
		t.Error("a share with no recorded mount should refuse the swap")
	}
}

func TestVMScripts(t *testing.T) {
	ms := []Mount{{Target: "/Users/me", Port: 58218}, {Target: "/mnt/it's", Port: 58219}}
	if got, want := UnmountScript(ms), `sudo umount -l '/Users/me' '/mnt/it'\''s' 2>/dev/null; true`; got != want {
		t.Errorf("UnmountScript = %s", got)
	}
	if got, want := MountScript(ms), `{ mountpoint -q '/Users/me' || sudo podman machine client9p 58218 '/Users/me'; } && { mountpoint -q '/mnt/it'\''s' || sudo podman machine client9p 58219 '/mnt/it'\''s'; }`; got != want {
		t.Errorf("MountScript = %s", got)
	}
}
