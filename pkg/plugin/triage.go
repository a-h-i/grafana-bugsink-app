// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const (
	msgCallerRefused = "Only editors and admins can triage issues."
	msgTriageOff     = "Triage is switched off for this data source."
	msgTriageHeader  = "Triage calls need the header X-Bugsink-Triage: 1."
	triageHeader     = "X-Bugsink-Triage"
)

// callerMayTriage reports whether the user's org role is Editor or Admin.
func callerMayTriage(u *backend.User) bool {
	return u != nil && (u.Role == "Editor" || u.Role == "Admin")
}

// handleTriage checks the caller, the settings, the switch, the triage header and the id in that order, then runs action on the issue.
func (d *Datasource) handleTriage(action bugsink.TriageAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx := r.Context()
		user := backend.UserFromContext(ctx)
		if !callerMayTriage(user) {
			writeMessage(w, http.StatusForbidden, msgCallerRefused)
			return
		}
		if !d.settingsValid(w) {
			return
		}
		if !d.triageEnabled {
			writeMessage(w, http.StatusForbidden, msgTriageOff)
			return
		}
		if r.Header.Get(triageHeader) != "1" {
			writeMessage(w, http.StatusForbidden, msgTriageHeader)
			return
		}
		id, ok := issueID(w, r)
		if !ok {
			return
		}
		issue, err := d.client.Triage(ctx, id, action)
		d.issueCache.clear()
		backend.Logger.FromContext(ctx).Info("Triage", "action", string(action), "issue", id, "user", user.Login, "status", bugsinkStatus(err))
		if err != nil {
			writeBugsinkError(w, err)
			return
		}
		project, ok := d.projectNames(ctx)[issue.ProjectID]
		if !ok {
			project = strconv.Itoa(issue.ProjectID)
		}
		writeJSON(w, http.StatusOK, issueJSON(issue, project, d.browserURL))
	}
}

// bugsinkStatus is the HTTP status Bugsink answered a triage call with, or 0 when it gave none.
func bugsinkStatus(err error) int {
	var apiErr *bugsink.APIError
	switch {
	case err == nil:
		return http.StatusOK
	case errors.As(err, &apiErr):
		return apiErr.Status
	default:
		return 0
	}
}
