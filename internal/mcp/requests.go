package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// execRequestList lists a site's recent requests with what went wrong in each,
// newest first; the request tool reads one of them by its id.
func execRequestList(args map[string]any) (any, *rpcError) {
	path := queryPath("/api/requests", [][2]string{{"site", strArg(args, "site")}, {"branch", strArg(args, "branch")}, {"limit", fmt.Sprint(intArg(args, "limit", 20))}})
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	var list []any
	_ = json.Unmarshal(body, &list)
	out := map[string]any{"requests": list}
	if len(list) == 0 {
		out["hint"] = "No requests captured. Debug capture must be on (dumps_toggle) for PHP requests to be recorded; then load a page or call the API."
	}
	b, _ := json.Marshal(out)
	return toolOK(string(b)), nil
}
