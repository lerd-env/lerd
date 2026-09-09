package config

import "testing"

// composer's home is a composer project like any other, so the sandbox project
// stands in for it: a manifest naming what `composer global require` installed.
func TestGlobalHostBinary(t *testing.T) {
	_, composerHome := packageSandbox(t, []string{`{"name":"acme/cloud-cli"}`}, "acme/cloud-cli")
	writePackage(t, "acme-cloud-cli", "package: acme/cloud-cli\nhost_binaries:\n  - cloud\n")

	if !GlobalHostBinary(composerHome, "cloud") {
		t.Error("a declared global binary must run on the host")
	}
	if GlobalHostBinary(composerHome, "psysh") {
		t.Error("a binary no package declares must stay in the container")
	}
}

func TestGlobalHostBinary_packageNotInstalledGlobally(t *testing.T) {
	_, composerHome := packageSandbox(t, []string{`{"name":"acme/cloud-cli"}`})
	writePackage(t, "acme-cloud-cli", "package: acme/cloud-cli\nhost_binaries:\n  - cloud\n")

	if GlobalHostBinary(composerHome, "cloud") {
		t.Error("a package this machine never installed must not answer for a binary name")
	}
}

// The index is what the machine is allowed to read, the same as for a project's
// package layer, so a file left behind by an earlier catalogue cannot keep
// routing a binary out of the container.
func TestGlobalHostBinary_packageOutsideTheIndexIsIgnored(t *testing.T) {
	_, composerHome := packageSandbox(t, nil, "acme/cloud-cli")
	writePackage(t, "acme-cloud-cli", "package: acme/cloud-cli\nhost_binaries:\n  - cloud\n")

	if GlobalHostBinary(composerHome, "cloud") {
		t.Error("package outside the index must not be consulted")
	}
}

// A package whose only declaration is a host binary contributes nothing to a
// framework, so without this the listing shows it as declaring nothing at all.
func TestListStorePackages_reportsHostBinaries(t *testing.T) {
	_, composerHome := packageSandbox(t, []string{`{"name":"acme/cloud-cli"}`}, "acme/cloud-cli")
	writePackage(t, "acme-cloud-cli", "package: acme/cloud-cli\nhost_binaries:\n  - cloud\n")

	got := ListStorePackages(composerHome)
	if len(got) != 1 {
		t.Fatalf("listed %d packages, want 1", len(got))
	}
	if len(got[0].HostBinaries) != 1 || got[0].HostBinaries[0] != "cloud" {
		t.Errorf("host binaries = %v, want [cloud]", got[0].HostBinaries)
	}
}
