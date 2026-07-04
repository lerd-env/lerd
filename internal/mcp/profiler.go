package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"github.com/geodro/lerd/internal/agentenv"
	"github.com/geodro/lerd/internal/config"
	phpDet "github.com/geodro/lerd/internal/php"
	"github.com/geodro/lerd/internal/podman"
	"github.com/geodro/lerd/internal/profiler"
)

func execProfilerToggle(args map[string]any) (any, *rpcError) {
	enableRaw, ok := args["enable"]
	if !ok {
		return toolErr(`"enable" is required (true or false)`), nil
	}
	enable, ok := enableRaw.(bool)
	if !ok {
		return toolErr(`"enable" must be a boolean`), nil
	}
	res, err := profiler.SetProfiling(enable)
	if err != nil {
		return toolErr("toggle failed: " + err.Error()), nil
	}
	b, _ := json.Marshal(res)
	return toolOK(string(b)), nil
}

func execProfilerStatus(_ map[string]any) (any, *rpcError) {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return toolErr(err.Error()), nil
	}
	snap := map[string]any{
		"enabled":    cfg.IsProfilerEnabled(),
		"spx_ui_url": profiler.SpxUIURL,
	}
	b, _ := json.Marshal(snap)
	return toolOK(string(b)), nil
}

func execProfilerClear(_ map[string]any) (any, *rpcError) {
	removed, err := profiler.ClearData()
	if err != nil {
		return toolErr("clear failed: " + err.Error()), nil
	}
	b, _ := json.Marshal(map[string]int{"removed": removed})
	return toolOK(string(b)), nil
}

// execProfilerReport runs a PHP command under SPX with the flat-profile report,
// returning the top functions by wall time and call count as text. This is the
// CPU-bound analog of analyze_queries: when a route is slow but the cost is not in
// its queries, this hands back the functions behind it instead of a browser URL.
// It profiles a reproducible command (an artisan call, a repro script), not a live
// HTTP request, which is the case SPX can report as text.
func execProfilerReport(args map[string]any) (any, *rpcError) {
	projectPath := resolvedPath(args)
	if s := strArg(args, "site"); s != "" {
		site, err := config.FindSiteByRef(s)
		if err != nil {
			return toolErr("site not found: " + s), nil
		}
		projectPath = site.Path
	}
	if projectPath == "" {
		return toolErr("provide a site or path, or open the assistant in the project directory"), nil
	}
	argv := strSliceArg(args, "args")
	if len(argv) == 0 {
		return toolErr(`args is required: the argv to run under php and profile, e.g. ["artisan","app:import"] or ["repro.php"]`), nil
	}

	phpVersion, err := phpDet.DetectVersion(projectPath)
	if err != nil {
		cfg, cfgErr := config.LoadGlobal()
		if cfgErr != nil {
			return toolErr("failed to detect PHP version: " + err.Error()), nil
		}
		phpVersion = cfg.PHP.DefaultVersion
	}
	short := strings.ReplaceAll(phpVersion, ".", "")
	container := "lerd-php" + short + "-fpm"
	if errBody := ensureFPMStartedMCP(phpVersion, short, container); errBody != nil {
		return errBody, nil
	}

	// SPX_REPORT=fp prints a flat profile to the run's output instead of storing it
	// for the web UI, which is the machine-readable form an agent can act on.
	cmdArgs := []string{"exec", "-w", projectPath, "--env", "SPX_ENABLED=1", "--env", "SPX_REPORT=fp"}
	for _, e := range agentenv.MCPInject(os.Environ()) {
		cmdArgs = append(cmdArgs, "--env", e)
	}
	cmdArgs = append(cmdArgs, container, "php")
	cmdArgs = append(cmdArgs, argv...)

	var out bytes.Buffer
	cmd := podman.Cmd(cmdArgs...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()

	report := extractSpxReport(stripANSI(out.String()))
	if !strings.Contains(report, "SPX Report") {
		msg := "no SPX profile was produced (profiling may be disabled, or the command exited before doing any work)"
		if runErr != nil {
			msg += ": " + runErr.Error()
		}
		return toolErr(msg + "\n" + strings.TrimSpace(out.String())), nil
	}

	exitCode := 0
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	b, _ := json.Marshal(map[string]any{
		"command":   "php " + strings.Join(argv, " "),
		"exit_code": exitCode,
		"report":    report,
	})
	return toolOK(string(b)), nil
}

// extractSpxReport slices SPX's flat-profile report out of a run's combined
// output, dropping the command's own stdout that precedes it. Returns the trimmed
// whole output when the report marker is absent, so a caller can tell SPX did not
// run rather than seeing an empty string.
func extractSpxReport(out string) string {
	if i := strings.Index(out, "*** SPX Report ***"); i >= 0 {
		return strings.TrimSpace(out[i:])
	}
	return strings.TrimSpace(out)
}
