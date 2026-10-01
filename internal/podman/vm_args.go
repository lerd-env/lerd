package podman

import "github.com/geodro/lerd/internal/hostpath"

// mapVMArgs returns args with the working directory of an exec or run given a
// path the guest can hold. The caller's cwd is a host path, which on Windows
// (C:\Sites\app) does not exist inside the Linux VM; hostpath.ToVM leaves every
// other value alone, so this is the identity on Linux and macOS. The input is
// not modified.
func mapVMArgs(args []string) []string {
	if len(args) == 0 || (args[0] != "exec" && args[0] != "run") {
		return args
	}
	out := args
	for i := 1; i+1 < len(args); i++ {
		if args[i] != "-w" && args[i] != "--workdir" {
			continue
		}
		mapped := hostpath.ToVM(args[i+1])
		if mapped == args[i+1] {
			continue
		}
		if &out[0] == &args[0] {
			out = append([]string(nil), args...)
		}
		out[i+1] = mapped
	}
	return out
}
