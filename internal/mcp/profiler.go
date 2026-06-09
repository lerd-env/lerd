package mcp

import (
	"encoding/json"

	"github.com/geodro/lerd/internal/config"
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
