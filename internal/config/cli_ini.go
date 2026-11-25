package config

import "sort"

// The CLI SAPI never reads a project's .user.ini, so a framework whose commands
// exhaust PHP's 128M default declares php.cli_ini, and lerd passes it as -d on
// every PHP process it starts for that project.

// CLIIniArgs renders the framework's cli_ini as `-d name=value` pairs, sorted
// so the argv is stable. An invalid set yields nothing rather than letting a
// value smuggle a second -d through.
func CLIIniArgs(fw *Framework) []string {
	if fw == nil || len(fw.PHP.CLIIni) == 0 {
		return nil
	}
	if err := ValidatePHPIni(fw.PHP.CLIIni); err != nil {
		return nil
	}
	keys := make([]string, 0, len(fw.PHP.CLIIni))
	for k := range fw.PHP.CLIIni {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		args = append(args, "-d", k+"="+fw.PHP.CLIIni[k])
	}
	return args
}

// CLIIniArgsForDir resolves the project's framework and renders its cli_ini.
func CLIIniArgsForDir(dir string) []string {
	name, ok := DetectFrameworkForDir(dir)
	if !ok {
		return nil
	}
	fw, ok := GetFrameworkForDir(name, dir)
	if !ok {
		return nil
	}
	return CLIIniArgs(fw)
}
