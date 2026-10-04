package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// execRequest lists a site's recent requests with what went wrong in each, or
// with rid returns one request and everything that carried its id: queries
// with runnable examples, exceptions, logs, dumps, components, browser events,
// the page view that sent it and the requests it sent.
func execRequest(args map[string]any) (any, *rpcError) {
	rid := strArg(args, "rid")
	path := "/api/requests/" + url.PathEscape(rid)
	if rid == "" {
		limit := "20"
		if v, ok := args["limit"]; ok {
			limit = fmt.Sprintf("%v", v)
		}
		path = queryPath("/api/requests", [][2]string{{"site", strArg(args, "site")}, {"branch", strArg(args, "branch")}, {"limit", limit}})
	}
	body, status, err := uiGET(path)
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status == http.StatusNotFound {
		return toolErr(fmt.Sprintf("no request %q in the buffer; list requests without rid to see what is there", rid)), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	if rid == "" {
		var list []any
		_ = json.Unmarshal(body, &list)
		out := map[string]any{"requests": list}
		if len(list) == 0 {
			out["hint"] = "No requests captured. Debug capture must be on (dumps_toggle) for PHP requests to be recorded; then load a page or call the API."
		}
		b, _ := json.Marshal(out)
		return toolOK(string(b)), nil
	}
	var detail map[string]any
	if err := json.Unmarshal(body, &detail); err != nil {
		return toolErr("decoding request: " + err.Error()), nil
	}
	// The events keep what says what happened; the context every one of them
	// repeats is already in the request's own fields.
	if events, ok := detail["events"].(map[string]any); ok {
		for kind, list := range events {
			items, _ := list.([]any)
			slim := make([]any, 0, len(items))
			for _, it := range items {
				e, _ := it.(map[string]any)
				if e == nil {
					continue
				}
				s := map[string]any{"ts": e["ts"]}
				for _, k := range []string{"label", "text", "data"} {
					if v, ok := e[k]; ok && v != nil {
						s[k] = v
					}
				}
				if src, ok := e["src"].(map[string]any); ok && src["file"] != "" && src["file"] != nil {
					s["at"] = fmt.Sprintf("%v:%v", src["file"], src["line"])
				}
				slim = append(slim, s)
			}
			events[kind] = slim
		}
	}
	b, _ := json.Marshal(detail)
	return toolOK(string(b)), nil
}
