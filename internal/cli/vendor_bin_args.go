package cli

import (
	"slices"

	"github.com/geodro/lerd/internal/config"
)

// vendorBinDefaultArgs returns the arguments the project's framework wants in
// front of the composer binary called name. Resolved from the framework
// definition only: a project .lerd.yaml is untrusted, and the sanitiser drops
// vendor_bin_args from an embedded definition for that reason.
func vendorBinDefaultArgs(cwd, name string) []string {
	root := projectRootFromCwd(cwd)
	fwName, ok := config.DetectFrameworkForDir(root)
	if !ok {
		return nil
	}
	fw, ok := config.GetFrameworkForDir(fwName, root)
	if !ok || fw == nil {
		return nil
	}
	return fw.VendorBinArgs[name]
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
