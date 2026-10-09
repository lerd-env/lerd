package podman

import "github.com/geodro/lerd/internal/hostpath"

// mapVMArgs returns the args of an exec or run with every host path given as
// the path the guest holds it at. The caller's cwd, the script and the paths a
// command works on are host paths, which on Windows (C:\Sites\app) do not exist
// inside the Linux VM. A volume source is left alone, since podman resolves it
// on the host side. hostpath.ToVM leaves every other value as it is, so this is
// the identity on Linux and macOS. The input is not modified.
func mapVMArgs(args []string) []string {
	if len(args) == 0 || (args[0] != "exec" && args[0] != "run") {
		return args
	}
	out := args
	for i := 1; i < len(args); i++ {
		if args[i] == "-v" || args[i] == "--volume" {
			i++
			continue
		}
		mapped := hostpath.RelToVM(hostpath.ToVM(args[i]))
		if mapped == args[i] {
			continue
		}
		if &out[0] == &args[0] {
			out = append([]string(nil), args...)
		}
		out[i] = mapped
	}
	return out
}

// withGuestEnv adds guestExecEnv to an exec, ahead of the caller's own --env
// flags so one the caller sets still wins. Anything else is returned as is.
func withGuestEnv(args []string) []string {
	if len(guestExecEnv) == 0 || len(args) == 0 || args[0] != "exec" {
		return args
	}
	out := make([]string, 0, len(args)+len(guestExecEnv))
	out = append(append(append(out, args[0]), guestExecEnv...), args[1:]...)
	return out
}
