// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const (
	msgCallerRefused  = "Only editors and admins can triage issues."
	msgTriageOff      = "Triage is switched off for this data source."
	msgTriageHeader   = "Triage calls need the header X-Bugsink-Triage: 1."
	msgMuteForShape   = "The request body must be a JSON object with period and periods."
	msgMuteUntilShape = "The request body must be a JSON object with period, periods and threshold."
	msgPeriod         = "The period must be minute, hour, day, week, month or year."
	msgCommentFailed  = "The action succeeded, but the attribution comment was not written: "
	triageHeader      = "X-Bugsink-Triage"
	maxTriageBody     = 1024
	maxLoginRunes     = 100
)

var (
	msgBodyTooLarge = fmt.Sprintf("The request body is larger than %d bytes.", maxTriageBody)
	msgPeriods      = fmt.Sprintf("The number of periods must be a whole number from 1 to %d.", bugsink.MaxPeriods)
	msgThreshold    = fmt.Sprintf("The threshold must be a whole number from 1 to %d.", bugsink.MaxThreshold)
)

// triageBody is the issue after an action, with a warning when the attribution comment failed.
type triageBody struct {
	issueBody
	Warning string `json:"warning,omitempty"`
}

// callerMayTriage reports whether the user's org role is Editor or Admin.
func callerMayTriage(u *backend.User) bool {
	return u != nil && (u.Role == "Editor" || u.Role == "Admin")
}

// handleTriage checks the caller, the settings, the switch, the triage header, the id and the mute terms in that order, then runs action on the issue and attributes it.
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
		var terms bugsink.MuteTerms
		if action == bugsink.MuteFor || action == bugsink.MuteUntil {
			var refusal string
			if terms, refusal = readMuteTerms(r, action); refusal != "" {
				writeMessage(w, http.StatusBadRequest, refusal)
				return
			}
		}
		issue, err := d.client.Triage(ctx, id, action, terms)
		d.issueCache.clear()
		logArgs := []any{"action", string(action), "issue", id, "user", user.Login, "role", user.Role, "status", bugsinkStatus(err)}
		if err != nil {
			backend.Logger.FromContext(ctx).Info("Triage", logArgs...)
			writeBugsinkError(w, err)
			return
		}
		var body triageBody
		if d.attribution {
			var commentStatus int
			commentStatus, body.Warning = d.attribute(ctx, issue.ID, attributionText(action, terms, user.Login))
			logArgs = append(logArgs, "comment", commentStatus)
		}
		backend.Logger.FromContext(ctx).Info("Triage", logArgs...)
		project, ok := d.projectNames(ctx)[issue.ProjectID]
		if !ok {
			project = strconv.Itoa(issue.ProjectID)
		}
		body.issueBody = issueJSON(issue, project, d.browserURL)
		writeJSON(w, http.StatusOK, body)
	}
}

// attribute adds text as a comment to the issue and returns Bugsink's status for it and, when it failed, the warning.
func (d *Datasource) attribute(ctx context.Context, issueID, text string) (int, string) {
	status, err := d.client.AddComment(ctx, issueID, text)
	if err == nil {
		return status, ""
	}
	message, _ := userMessage(err)
	return bugsinkStatus(err), msgCommentFailed + message
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

// readMuteTerms decodes and validates the body of a mute route, or returns the refusal message.
func readMuteTerms(r *http.Request, action bugsink.TriageAction) (bugsink.MuteTerms, string) {
	withThreshold := action == bugsink.MuteUntil
	shape, keys := msgMuteForShape, []string{"period", "periods"}
	if withThreshold {
		shape, keys = msgMuteUntilShape, append(keys, "threshold")
	}
	if r.Body == nil {
		return bugsink.MuteTerms{}, shape
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxTriageBody+1))
	if err != nil {
		return bugsink.MuteTerms{}, shape
	}
	if len(raw) > maxTriageBody {
		return bugsink.MuteTerms{}, msgBodyTooLarge
	}
	fields, ok := objectFields(raw)
	if !ok || len(fields) != len(keys) {
		return bugsink.MuteTerms{}, shape
	}
	for _, k := range keys {
		if _, ok := fields[k]; !ok {
			return bugsink.MuteTerms{}, shape
		}
	}
	var period *string
	periods, periodsOK := jsonInteger(fields["periods"])
	threshold, thresholdOK := 0, true
	if withThreshold {
		threshold, thresholdOK = jsonInteger(fields["threshold"])
	}
	if json.Unmarshal(fields["period"], &period) != nil || period == nil || !periodsOK || !thresholdOK {
		return bugsink.MuteTerms{}, shape
	}
	terms := bugsink.MuteTerms{Period: bugsink.Period(*period), Periods: periods, Threshold: threshold}
	switch {
	case !terms.Period.Valid():
		return bugsink.MuteTerms{}, msgPeriod
	case periods < 1 || periods > bugsink.MaxPeriods:
		return bugsink.MuteTerms{}, msgPeriods
	case withThreshold && (threshold < 1 || threshold > bugsink.MaxThreshold):
		return bugsink.MuteTerms{}, msgThreshold
	}
	return terms, ""
}

// objectFields splits raw into the members of one JSON object, refusing a repeated key or anything after the object.
func objectFields(raw []byte) (map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		key, isKey := tok.(string)
		if err != nil || !isKey {
			return nil, false
		}
		if _, repeated := fields[key]; repeated {
			return nil, false
		}
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			return nil, false
		}
		fields[key] = value
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return fields, true
}

var integerLiteral = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// jsonInteger reads raw as a JSON number without a fraction or exponent; one too large for an int reads as -1.
func jsonInteger(raw json.RawMessage) (int, bool) {
	if !integerLiteral.Match(raw) {
		return 0, false
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil {
		return -1, true
	}
	return n, true
}

var actionDone = map[bugsink.TriageAction]string{
	bugsink.Resolve:       "Resolved",
	bugsink.ResolveLatest: "Resolved in the latest release",
	bugsink.ResolveNext:   "Resolved in the next release",
	bugsink.Reopen:        "Reopened",
	bugsink.Mute:          "Muted",
	bugsink.Unmute:        "Unmuted",
}

// attributionText is the comment naming action, its terms and the Grafana login that took it.
func attributionText(action bugsink.TriageAction, terms bugsink.MuteTerms, login string) string {
	what := actionDone[action]
	switch action {
	case bugsink.MuteFor:
		what = "Muted for " + periodsText(terms)
	case bugsink.MuteUntil:
		what = fmt.Sprintf("Muted until %d or more events in %s", terms.Threshold, periodsText(terms))
	}
	return what + " from Grafana by " + author(login) + "."
}

// periodsText is the number of periods and the unit, singular for 1.
func periodsText(t bugsink.MuteTerms) string {
	if t.Periods == 1 {
		return "1 " + string(t.Period)
	}
	return fmt.Sprintf("%d %ss", t.Periods, t.Period)
}

// author is login in double quotes on one line, with its quotation marks made apostrophes and cut to maxLoginRunes, or "an anonymous user" when it is empty.
func author(login string) string {
	if login == "" {
		return "an anonymous user"
	}
	var b strings.Builder
	b.WriteByte('"')
	n := 0
	for _, r := range login {
		if n == maxLoginRunes {
			b.WriteString("…")
			break
		}
		switch {
		case unicode.Is(unicode.Quotation_Mark, r):
			r = '\''
		case unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp):
			r = unicode.ReplacementChar
		}
		b.WriteRune(r)
		n++
	}
	b.WriteByte('"')
	return b.String()
}
