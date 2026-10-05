package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func annotationTool() mcpTool {
	return mcpTool{
		Name:        "annotation",
		Description: "Notes pinned to page elements from the debug bar: selector, text, page view. action: list (open notes), get, resolve (once fixed).",
		InputSchema: mcpSchema{
			Type: "object",
			Properties: map[string]mcpProp{
				"action":     {Type: "string", Enum: []string{"list", "get", "resolve"}},
				"site":       {Type: "string", Description: "Site name."},
				"id":         {Type: "string", Description: "get/resolve: note id."},
				"status":     {Type: "string", Description: "list: open, resolved or all."},
				"resolution": {Type: "string", Description: "resolve: what was done."},
			},
			Required: []string{"action"},
		},
	}
}

func execAnnotationTool(args map[string]any) (any, *rpcError) {
	site, id := strArg(args, "site"), strArg(args, "id")
	if site == "" {
		site = siteForToolArgs(args)
	}
	if site == "" {
		return toolErr("site is required"), nil
	}
	action := strArg(args, "action")
	if action != "list" && id == "" {
		return toolErr("id is required; list the site's notes to find one"), nil
	}
	var body []byte
	var status int
	var err error
	switch action {
	case "list":
		body, status, err = uiGET(queryPath("/api/annotations", [][2]string{{"site", site}, {"status", strArg(args, "status")}}))
	case "get":
		body, status, err = uiGET("/api/annotations/" + url.PathEscape(site) + "/" + url.PathEscape(id))
	case "resolve":
		req, _ := json.Marshal(map[string]string{"status": "resolved", "resolution": strArg(args, "resolution")})
		body, status, err = uiPOST("/api/annotations/"+url.PathEscape(site)+"/"+url.PathEscape(id), req)
	default:
		return toolErr(fmt.Sprintf("unknown action %q (want list, get or resolve)", action)), nil
	}
	if err != nil {
		return toolErr("lerd-ui not reachable: " + err.Error()), nil
	}
	if status != http.StatusOK {
		return toolErr(fmt.Sprintf("lerd-ui returned %d: %s", status, body)), nil
	}
	if action != "get" {
		return toolOK(string(body)), nil
	}
	var note map[string]any
	_ = json.Unmarshal(body, &note)
	if rid, _ := note["rid"].(string); rid != "" {
		note["hint"] = fmt.Sprintf("The page it was made on is request %s: read it with the request tool (summary, then tabs).", rid)
	}
	return toolJSON(note), nil
}
