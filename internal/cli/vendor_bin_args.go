package cli

import (
	"fmt"
	"slices"

	"github.com/geodro/lerd/internal/config"
)

// vendorBinDefaultArgs returns the arguments the project's framework wants in
// front of the composer binary called name. Resolved from the framework
// definition only: a project .lerd.yaml is untrusted, and the sanitiser drops
// vendor_bin_args from an embedded definition for that reason.
func vendorBinDefaultArgs(cwd, name string) []string {
	fw := trustedFrameworkForDir(projectRootFromCwd(cwd))
	if fw == nil {
		return nil
	}
	return fw.VendorBinArgs[name]
}

// trustedFrameworkForDir resolves the framework of the project at root, nil
// when it has none. The definition comes from the store or a user overlay.
func trustedFrameworkForDir(root string) *config.Framework {
	fwName, ok := config.DetectFrameworkForDir(root)
	if !ok {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(fwName, root)
	if !ok {
		return nil
	}
	return fw
}

// applyVendorBinDefaults puts defaults in front of the user's arguments, the
// position wp-cli and friends read global parameters from. A default the user
// already typed is left alone, so their invocation stays exactly as written.
func applyVendorBinDefaults(defaults, args []string) []string {
	var add []string
	for _, d := range defaults {
		if !slices.Contains(args, d) {
			add = append(add, d)
		}
	}
	if len(add) == 0 {
		return args
	}
	return append(add, args...)
}

// MissingVendorBin explains a composer binary the project's framework runs
// through vendor_bin_args but the project has not installed, so `lerd wp` on a
// site without wp-cli says what to do instead of "unknown command".
func MissingVendorBin(cwd, name string) error {
	root := projectRootFromCwd(cwd)
	fw := trustedFrameworkForDir(root)
	if fw == nil || HasVendorBinTool(root, name) {
		return nil
	}
	if _, declared := fw.VendorBinArgs[name]; !declared || VendorBinExists(root, name) {
		return nil
	}
	return fmt.Errorf("%s is not installed: this %s site runs it from vendor/bin/%s, which is missing; install the composer package that provides it with lerd composer require, then run lerd %s again", name, fw.Label, name, name)
}
