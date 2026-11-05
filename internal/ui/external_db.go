package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/geodro/lerd/internal/serviceops"
)

type externalDBRequest struct {
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

func (r externalDBRequest) validate() string {
	switch {
	case r.Engine == "" || r.Host == "" || r.User == "":
		return "engine, host and user are required"
	case r.Port < 1 || r.Port > 65535:
		return "port must be between 1 and 65535"
	}
	return ""
}

func (r externalDBRequest) service() serviceops.ExternalRequest {
	return serviceops.ExternalRequest{Name: r.Name, Engine: r.Engine, Host: strings.TrimSpace(r.Host), Port: r.Port, User: r.User, Password: r.Password}
}

// handleExternalDB tests or adds a server lerd does not run. A failure answers
// 400 with the client's own reason, which is what the form shows.
func handleExternalDB(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req externalDBRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if msg := req.validate(); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/test") {
		version, err := serviceops.TestExternalDB(req.service())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"version": version})
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	svc, err := serviceops.AddExternalDB(req.service())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"name": svc.Name})
}
