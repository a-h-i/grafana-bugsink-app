// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/google/uuid"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// maxTagValues is the number of values listed per tag key.
const maxTagValues = 5

var issueIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// validIssueID reports whether id is a uuid or a short id.
func validIssueID(id string) bool { return issueIDPattern.MatchString(id) }

// CallResource serves the data source's resource calls.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return d.resources.CallResource(ctx, req, sender)
}

// routes builds the resource router; reads answer GET only, triage actions POST only.
func (d *Datasource) routes() *http.ServeMux {
	mux := http.NewServeMux()
	get := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc("GET "+pattern, h)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Allow", http.MethodGet)
			writeMessage(w, http.StatusMethodNotAllowed, "Method not allowed.")
		})
	}
	post := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc("POST "+pattern, h)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Allow", http.MethodPost)
			writeMessage(w, http.StatusMethodNotAllowed, "Method not allowed.")
		})
	}
	get("/projects", d.handleProjects)
	get("/issues/{id}", d.handleIssue)
	get("/issues/{id}/tags", d.handleIssueTags)
	get("/issues/{id}/history", d.handleIssueHistory)
	get("/events/{id}", d.handleEvent)
	for _, action := range []bugsink.TriageAction{bugsink.Resolve, bugsink.Reopen, bugsink.Mute, bugsink.Unmute} {
		post("/issues/{id}/"+string(action), d.handleTriage(action))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeMessage(w, http.StatusNotFound, "Not found.")
	})
	return mux
}

// settingsValid answers 400 with the settings message and returns false when the settings are invalid.
func (d *Datasource) settingsValid(w http.ResponseWriter) bool {
	if d.settingsErr != "" {
		writeMessage(w, http.StatusBadRequest, d.settingsErr)
		return false
	}
	return true
}

// issueID returns the validated {id} of the request, or answers 400 and returns false.
func issueID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !validIssueID(id) {
		writeMessage(w, http.StatusBadRequest, notAnIssueID(id))
		return "", false
	}
	return id, true
}

// issueRequest checks the settings, then returns the validated issue id.
func (d *Datasource) issueRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !d.settingsValid(w) {
		return "", false
	}
	return issueID(w, r)
}

func notAnIssueID(id string) string { return fmt.Sprintf("%q is not an issue id.", id) }

func notAnEventID(id string) string { return fmt.Sprintf("%q is not an event id.", id) }

// canonical answers a path with an empty or dot segment as JSON; the mux would redirect it.
func canonical(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		segments := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
		for _, raw := range segments {
			segment, err := url.PathUnescape(raw)
			if err != nil || segment == "" || segment == "." || segment == ".." {
				switch segments[0] {
				case "issues":
					writeMessage(w, http.StatusBadRequest, notAnIssueID(segment))
				case "events":
					writeMessage(w, http.StatusBadRequest, notAnEventID(segment))
				default:
					writeMessage(w, http.StatusNotFound, "Not found.")
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeBugsinkError(w http.ResponseWriter, err error) {
	message, _ := userMessage(err)
	writeMessage(w, http.StatusBadGateway, message)
}

func (d *Datasource) handleProjects(w http.ResponseWriter, r *http.Request) {
	if !d.settingsValid(w) {
		return
	}
	projects, err := d.projects(r.Context())
	if err != nil {
		writeBugsinkError(w, err)
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

type issueBody struct {
	ID                      string     `json:"id"`
	FriendlyID              string     `json:"friendlyId"`
	ProjectID               int        `json:"projectId"`
	Project                 string     `json:"project"`
	Type                    string     `json:"type"`
	Message                 string     `json:"message"`
	Transaction             string     `json:"transaction"`
	Status                  string     `json:"status"`
	FirstSeen               time.Time  `json:"firstSeen"`
	LastSeen                time.Time  `json:"lastSeen"`
	Events                  int        `json:"events"`
	StoredEvents            int        `json:"storedEvents"`
	IsResolved              bool       `json:"isResolved"`
	IsResolvedByNextRelease bool       `json:"isResolvedByNextRelease"`
	IsMuted                 bool       `json:"isMuted"`
	UnmuteAfter             *time.Time `json:"unmuteAfter"`
	BugsinkURL              string     `json:"bugsinkUrl"`
}

func issueJSON(i bugsink.Issue, project, browserURL string) issueBody {
	return issueBody{
		ID:                      i.ID,
		FriendlyID:              i.FriendlyID,
		ProjectID:               i.ProjectID,
		Project:                 project,
		Type:                    i.Type,
		Message:                 i.Value,
		Transaction:             i.Transaction,
		Status:                  statusOf(i),
		FirstSeen:               i.FirstSeen,
		LastSeen:                i.LastSeen,
		Events:                  i.DigestedEventCount,
		StoredEvents:            i.StoredEventCount,
		IsResolved:              i.IsResolved,
		IsResolvedByNextRelease: i.IsResolvedByNextRelease,
		IsMuted:                 i.IsMuted,
		UnmuteAfter:             i.UnmuteAfter,
		BugsinkURL:              browserURL + "/issues/issue/" + i.ID + "/event/last/",
	}
}

func (d *Datasource) handleIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := d.issueRequest(w, r)
	if !ok {
		return
	}
	issue, err := d.client.GetIssue(r.Context(), id)
	if err != nil {
		writeBugsinkError(w, err)
		return
	}
	project, ok := d.projectNames(r.Context())[issue.ProjectID]
	if !ok {
		project = strconv.Itoa(issue.ProjectID)
	}
	writeJSON(w, http.StatusOK, issueJSON(issue, project, d.browserURL))
}

type tagValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type tagGroup struct {
	Key         string     `json:"key"`
	Values      []tagValue `json:"values"`
	OtherValues int        `json:"otherValues"`
}

// groupTags groups values by key in order of first appearance, keeping the maxTagValues most frequent per key.
func groupTags(tags []bugsink.IssueTag) []tagGroup {
	groups := []tagGroup{}
	index := map[string]int{}
	all := map[string][]tagValue{}
	for _, t := range tags {
		if _, ok := index[t.Key]; !ok {
			index[t.Key] = len(groups)
			groups = append(groups, tagGroup{Key: t.Key})
		}
		all[t.Key] = append(all[t.Key], tagValue{Value: t.Value, Count: t.Count})
	}
	for i := range groups {
		values := all[groups[i].Key]
		sort.SliceStable(values, func(a, b int) bool {
			if values[a].Count != values[b].Count {
				return values[a].Count > values[b].Count
			}
			return values[a].Value < values[b].Value
		})
		if len(values) > maxTagValues {
			groups[i].OtherValues = len(values) - maxTagValues
			values = values[:maxTagValues]
		}
		groups[i].Values = values
	}
	return groups
}

func (d *Datasource) handleIssueTags(w http.ResponseWriter, r *http.Request) {
	id, ok := d.issueRequest(w, r)
	if !ok {
		return
	}
	list, err := d.client.ListIssueTags(r.Context(), id)
	if err != nil {
		writeBugsinkError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Tags      []tagGroup `json:"tags"`
		Truncated bool       `json:"truncated"`
	}{groupTags(list.Tags), list.Truncated})
}

type historyEntry struct {
	ID        int       `json:"id"`
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	User      *string   `json:"user"`
	Comment   string    `json:"comment"`
}

func (d *Datasource) handleIssueHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := d.issueRequest(w, r)
	if !ok {
		return
	}
	h, err := d.client.ListIssueHistory(r.Context(), id)
	if err != nil {
		writeBugsinkError(w, err)
		return
	}
	entries := make([]historyEntry, 0, len(h.Entries))
	for _, e := range h.Entries {
		entry := historyEntry{ID: e.ID, Kind: e.Kind, Timestamp: e.Timestamp, Comment: e.Comment}
		if e.User != "" {
			entry.User = &e.User
		}
		entries = append(entries, entry)
	}
	writeJSON(w, http.StatusOK, struct {
		History   []historyEntry `json:"history"`
		Truncated bool           `json:"truncated"`
	}{entries, h.Truncated})
}

type keyValueBody struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type contextBody struct {
	Name   string         `json:"name"`
	Fields []keyValueBody `json:"fields"`
}

type frameBody struct {
	Filename    string   `json:"filename"`
	Function    string   `json:"function"`
	Module      string   `json:"module"`
	Lineno      *int     `json:"lineno"`
	Colno       *int     `json:"colno"`
	InApp       bool     `json:"inApp"`
	PreContext  []string `json:"preContext"`
	ContextLine *string  `json:"contextLine"`
	PostContext []string `json:"postContext"`
}

type exceptionBody struct {
	Type          string      `json:"type"`
	Value         string      `json:"value"`
	Module        string      `json:"module"`
	Mechanism     string      `json:"mechanism"`
	Handled       *bool       `json:"handled"`
	Frames        []frameBody `json:"frames"`
	FramesOmitted int         `json:"framesOmitted"`
}

type breadcrumbBody struct {
	Timestamp *time.Time     `json:"timestamp"`
	Type      string         `json:"type"`
	Category  string         `json:"category"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Data      []keyValueBody `json:"data"`
}

type eventBody struct {
	ID           string           `json:"id"`
	EventID      string           `json:"eventId"`
	IssueID      string           `json:"issueId"`
	ProjectID    int              `json:"projectId"`
	Project      string           `json:"project"`
	Timestamp    time.Time        `json:"timestamp"`
	IngestedAt   time.Time        `json:"ingestedAt"`
	Platform     string           `json:"platform"`
	Level        string           `json:"level"`
	Message      string           `json:"message"`
	Transaction  string           `json:"transaction"`
	Environment  string           `json:"environment"`
	Release      string           `json:"release"`
	TraceID      string           `json:"traceId"`
	Tags         []keyValueBody   `json:"tags"`
	Contexts     []contextBody    `json:"contexts"`
	Exceptions   []exceptionBody  `json:"exceptions"`
	Breadcrumbs  []breadcrumbBody `json:"breadcrumbs"`
	StacktraceMd *string          `json:"stacktraceMd"`
	Notices      []string         `json:"notices"`
	BugsinkURL   string           `json:"bugsinkUrl"`
	Links        []data.DataLink  `json:"links"`
}

func keyValuesJSON(list []bugsink.KeyValue) []keyValueBody {
	out := make([]keyValueBody, 0, len(list))
	for _, kv := range list {
		out = append(out, keyValueBody{Key: kv.Key, Value: kv.Value})
	}
	return out
}

func exceptionsJSON(list []bugsink.Exception) []exceptionBody {
	out := make([]exceptionBody, 0, len(list))
	for _, x := range list {
		frames := make([]frameBody, 0, len(x.Frames))
		for _, f := range x.Frames {
			frames = append(frames, frameBody{
				Filename: f.Filename, Function: f.Function, Module: f.Module, Lineno: f.Lineno, Colno: f.Colno,
				InApp: f.InApp, PreContext: orEmpty(f.PreContext), ContextLine: f.ContextLine, PostContext: orEmpty(f.PostContext),
			})
		}
		out = append(out, exceptionBody{
			Type: x.Type, Value: x.Value, Module: x.Module, Mechanism: x.Mechanism, Handled: x.Handled,
			Frames: frames, FramesOmitted: x.FramesOmitted,
		})
	}
	return out
}

func eventJSON(e bugsink.EventDetail, project, browserURL string, links linkSettings) eventBody {
	contexts := make([]contextBody, 0, len(e.Contexts))
	for _, c := range e.Contexts {
		contexts = append(contexts, contextBody{Name: c.Name, Fields: keyValuesJSON(c.Fields)})
	}
	breadcrumbs := make([]breadcrumbBody, 0, len(e.Breadcrumbs))
	for _, b := range e.Breadcrumbs {
		breadcrumbs = append(breadcrumbs, breadcrumbBody{
			Timestamp: b.Timestamp, Type: b.Type, Category: b.Category, Level: b.Level, Message: b.Message,
			Data: keyValuesJSON(b.Data),
		})
	}
	var stacktraceMd *string
	if e.StacktraceMD != "" {
		stacktraceMd = &e.StacktraceMD
	}
	eventLinks := []data.DataLink{}
	if traceIDPattern.MatchString(e.TraceID) {
		logsQuery := renderLogsQuery(links.logsQuery, links.vars(e.TraceID, project, &e))
		eventLinks = append(eventLinks, links.exploreLinks(e.TraceID, logsQuery, e.Timestamp.Add(-links.window), e.Timestamp.Add(links.window))...)
	}
	return eventBody{
		ID: e.ID, EventID: e.EventID, IssueID: e.IssueID, ProjectID: e.ProjectID, Project: project,
		Timestamp: e.Timestamp, IngestedAt: e.IngestedAt,
		Platform: e.Platform, Level: e.Level, Message: e.Message, Transaction: e.Transaction,
		Environment: e.Environment, Release: e.Release, TraceID: e.TraceID,
		Tags: keyValuesJSON(e.Tags), Contexts: contexts, Exceptions: exceptionsJSON(e.Exceptions),
		Breadcrumbs: breadcrumbs, StacktraceMd: stacktraceMd, Notices: eventNotices(e),
		BugsinkURL: browserURL + "/issues/issue/" + e.IssueID + "/event/" + e.ID + "/",
		Links:      eventLinks,
	}
}

// eventNotices lists what the parser cut from the event.
func eventNotices(e bugsink.EventDetail) []string {
	notices := []string{}
	if e.ExceptionsCut {
		notices = append(notices, fmt.Sprintf("Only the last %d exceptions of the chain are shown.", bugsink.MaxExceptions))
	}
	for _, x := range e.Exceptions {
		if x.FramesOmitted > 0 {
			notices = append(notices, fmt.Sprintf("Only the %d frames nearest the error are shown for %s.", bugsink.MaxFrames, x.Type))
		}
	}
	if e.BreadcrumbsCut {
		notices = append(notices, fmt.Sprintf("Only the newest %d breadcrumbs are shown.", bugsink.MaxBreadcrumbs))
	}
	if e.EntriesCut {
		notices = append(notices, fmt.Sprintf("Only the first %d tags, contexts or fields of each list are shown.", bugsink.MaxEntries))
	}
	if e.ValuesCut {
		notices = append(notices, fmt.Sprintf("Values longer than %d characters are cut.", bugsink.MaxValueLength))
	}
	if e.StacktraceMDCut {
		notices = append(notices, "The stack trace text is cut at 100 KB.")
	}
	return notices
}

func (d *Datasource) handleEvent(w http.ResponseWriter, r *http.Request) {
	if !d.settingsValid(w) {
		return
	}
	raw := r.PathValue("id")
	id, err := uuid.Parse(raw)
	if err != nil || len(raw) != len(id.String()) {
		writeMessage(w, http.StatusBadRequest, notAnEventID(raw))
		return
	}
	event, err := d.client.GetEvent(r.Context(), id)
	if err != nil {
		writeBugsinkError(w, err)
		return
	}
	project, ok := d.projectNames(r.Context())[event.ProjectID]
	if !ok {
		project = strconv.Itoa(event.ProjectID)
	}
	textHidden := d.hideStacktraceText && event.StacktraceMD != ""
	if textHidden {
		event.StacktraceMD, event.StacktraceMDCut = "", false
	}
	body := eventJSON(event, project, d.browserURL, d.links)
	if textHidden {
		body.Notices = append(body.Notices, "Bugsink's stack trace text is hidden for this data source.")
	}
	writeJSON(w, http.StatusOK, body)
}

func writeMessage(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
