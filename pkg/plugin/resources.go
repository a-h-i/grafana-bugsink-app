// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// CallResource serves the data source's resource calls.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects", d.handleProjects)
	mux.HandleFunc("/projects", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "Method not allowed."})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not found."})
	})
	return httpadapter.New(mux).CallResource(ctx, req, sender)
}

func (d *Datasource) handleProjects(w http.ResponseWriter, r *http.Request) {
	if d.settingsErr != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": d.settingsErr})
		return
	}
	projects, err := d.projects(r.Context())
	if err != nil {
		message, _ := userMessage(err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": message})
		return
	}
	type project struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	list := make([]project, 0, len(projects))
	for _, p := range projects {
		list = append(list, project{ID: p.ID, Name: p.Name})
	}
	writeJSON(w, http.StatusOK, list)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
