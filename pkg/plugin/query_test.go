// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var (
	rangeFrom = time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	rangeTo   = time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
)

func issueAt(friendly string, lastSeen time.Time) string {
	return fmt.Sprintf(`{"id":"0b1e1c9e-1a2b-4c3d-8e4f-0123456789ab","friendly_id":%q,"project":1,`+
		`"digest_order":1,"first_seen":"2026-01-01T00:00:00Z","last_seen":%q,"digested_event_count":3,`+
		`"stored_event_count":2,"calculated_type":"ValueError","calculated_value":"boom","transaction":"/x",`+
		`"is_resolved":false,"is_resolved_by_next_release":false,"is_muted":false,"unmute_after":null}`,
		friendly, lastSeen.Format(time.RFC3339))
}

// fakeBugsink serves projects/ and issues/ and records every issue list request.
type fakeBugsink struct {
	mu             sync.Mutex
	requests       []url.Values
	pages          []string // result arrays of the issue list pages, by index
	endless        bool     // every page has a successor
	issuesStatus   int
	issuesBody     string
	projectsStatus int
	projectsBody   string // result objects of the project list, or empty for one project
	projectCalls   int
	errorBody      string // body of a failing project list, or empty for a bare detail
	paths          []string
	issueBody      string              // body of the issue detail, or empty for a default issue
	tagsBody       string              // result objects of the tag list
	tagsEndless    bool                // every tag page has a successor
	historyBody    string              // result objects of the history list
	detailStatus   int                 // status of every detail endpoint, 0 for success
	detailError    string              // body of a failing detail endpoint
	issueBodies    map[string]string   // issue detail bodies by the id in the path
	eventPages     map[string][]string // result objects of each event list page, by issue uuid
	eventsEndless  bool                // every event page has a successor
	eventsStatus   int                 // status of the event list, 0 for success
	eventRequests  []url.Values
	lookupRequests []url.Values
	lookupStatus   int               // status of the event lookup, 0 for success
	lookupBody     string            // body of the event lookup, or empty for a default event
	eventDetails   map[string]string // event detail bodies by event uuid
	eventStatus    int               // status of the event detail, 0 for success
	eventError     string            // body of a failing event detail
	srv            *httptest.Server
}

func newFakeBugsink(t *testing.T) *fakeBugsink {
	t.Helper()
	f := &fakeBugsink{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeBugsink) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	switch r.URL.Path {
	case "/api/canonical/0/projects/":
		f.mu.Lock()
		f.projectCalls++
		f.mu.Unlock()
		if f.projectsStatus != 0 {
			w.WriteHeader(f.projectsStatus)
			body := f.errorBody
			if body == "" {
				body = `{"detail":"nope"}`
			}
			_, _ = fmt.Fprint(w, body)
			return
		}
		results := f.projectsBody
		if results == "" {
			results = `{"id":1,"name":"alpha"}`
		}
		_, _ = fmt.Fprintf(w, `{"next":null,"previous":null,"results":[%s]}`, results)
	case "/api/canonical/0/issues/":
		f.mu.Lock()
		f.requests = append(f.requests, r.URL.Query())
		f.mu.Unlock()
		if f.issuesStatus != 0 {
			w.WriteHeader(f.issuesStatus)
			_, _ = fmt.Fprint(w, f.issuesBody)
			return
		}
		n := 0
		if c := r.URL.Query().Get("cursor"); c != "" {
			_, _ = fmt.Sscan(c, &n)
		}
		next := "null"
		if n+1 < len(f.pages) || f.endless {
			next = fmt.Sprintf("%q", fmt.Sprintf("%s/api/canonical/0/issues/?cursor=%d", f.srv.URL, n+1))
		}
		results := ""
		if n < len(f.pages) {
			results = f.pages[n]
		} else if f.endless {
			results = issueAt("E", rangeTo.Add(-time.Hour))
		}
		_, _ = fmt.Fprintf(w, `{"next":%s,"previous":null,"results":[%s]}`, next, results)
	case "/api/canonical/0/events/":
		f.serveEvents(w, r)
	case "/api/canonical/0/events/lookup/":
		f.serveLookup(w, r)
	default:
		if strings.HasPrefix(r.URL.Path, "/api/canonical/0/events/") {
			f.serveEventDetail(w, r)
			return
		}
		f.serveDetail(w, r)
	}
}

// serveEvents answers the event list of one issue and records the request.
func (f *fakeBugsink) serveEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	f.mu.Lock()
	f.eventRequests = append(f.eventRequests, query)
	status, pages, endless := f.eventsStatus, f.eventPages[query.Get("issue")], f.eventsEndless
	f.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, `{"detail":"nope"}`)
		return
	}
	n := 0
	if c := query.Get("cursor"); c != "" {
		_, _ = fmt.Sscan(c, &n)
	}
	next := "null"
	if n+1 < len(pages) || endless {
		next = fmt.Sprintf("%q", fmt.Sprintf("%s/api/canonical/0/events/?cursor=%d", f.srv.URL, n+1))
	}
	results := ""
	if n < len(pages) {
		results = pages[n]
	} else if endless {
		results = eventAt(defaultEventID, defaultEventSDKID, defaultIssueUUID, olderThanRange)
	}
	_, _ = fmt.Fprintf(w, `{"next":%s,"previous":null,"results":[%s]}`, next, results)
}

// serveLookup answers the event lookup and records the request.
func (f *fakeBugsink) serveLookup(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.lookupRequests = append(f.lookupRequests, r.URL.Query())
	status, body := f.lookupStatus, f.lookupBody
	f.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
	}
	if body == "" {
		body = eventAt(defaultEventID, defaultEventSDKID, defaultIssueUUID, olderThanRange)
	}
	_, _ = fmt.Fprint(w, body)
}

// serveEventDetail answers the detail of one event.
func (f *fakeBugsink) serveEventDetail(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	status, errBody := f.eventStatus, f.eventError
	f.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, errBody)
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/canonical/0/events/"), "/")
	f.mu.Lock()
	body, ok := f.eventDetails[id]
	f.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		body = `{"detail":"No Event matches the given query."}`
	}
	_, _ = fmt.Fprint(w, body)
}

// serveDetail answers the issue, tag and history reads.
func (f *fakeBugsink) serveDetail(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	isTags := strings.HasPrefix(path, "/api/canonical/0/issues/") && strings.HasSuffix(path, "/tags/")
	isIssue := strings.HasPrefix(path, "/api/canonical/0/issues/") && !isTags
	isHistory := path == "/api/canonical/0/issue-history/"
	if !isTags && !isIssue && !isHistory {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if f.detailStatus != 0 {
		w.WriteHeader(f.detailStatus)
		_, _ = fmt.Fprint(w, f.detailError)
		return
	}
	switch {
	case isIssue:
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/canonical/0/issues/"), "/")
		f.mu.Lock()
		body := f.issueBodies[id]
		f.mu.Unlock()
		if body == "" {
			body = f.issueBody
		}
		if body == "" {
			body = issueAt("SAMPLE-PYTHON-2", rangeTo)
		}
		_, _ = fmt.Fprint(w, body)
	case isTags:
		next := "null"
		if f.tagsEndless {
			n := 0
			if c := r.URL.Query().Get("cursor"); c != "" {
				_, _ = fmt.Sscan(c, &n)
			}
			next = fmt.Sprintf("%q", fmt.Sprintf("%s%s?cursor=%d", f.srv.URL, path, n+1))
		}
		_, _ = fmt.Fprintf(w, `{"next":%s,"previous":null,"results":[%s]}`, next, f.tagsBody)
	default:
		_, _ = fmt.Fprintf(w, `{"next":null,"previous":null,"results":[%s]}`, f.historyBody)
	}
}

func (f *fakeBugsink) setEventsStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eventsStatus = status
}

func (f *fakeBugsink) eventRequestList() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.eventRequests...)
}

func (f *fakeBugsink) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.paths)
}

func (f *fakeBugsink) pathCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, p := range f.paths {
		if p == path {
			n++
		}
	}
	return n
}

func (f *fakeBugsink) issueRequests() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.requests...)
}

func (f *fakeBugsink) projectRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.projectCalls
}

func (f *fakeBugsink) datasource(t *testing.T) *Datasource {
	return newInstance(t, f.srv.URL, secretToken, `{"browserUrl":"http://b"}`)
}

func runQueries(t *testing.T, ds *Datasource, from, to time.Time, models ...string) *backend.QueryDataResponse {
	t.Helper()
	req := &backend.QueryDataRequest{}
	for i, m := range models {
		req.Queries = append(req.Queries, backend.DataQuery{
			RefID: string(rune('A' + i)), JSON: json.RawMessage(m), TimeRange: backend.TimeRange{From: from, To: to},
		})
	}
	resp, err := ds.QueryData(context.Background(), req)
	if err != nil {
		t.Fatalf("QueryData returned error: %v", err)
	}
	return resp
}

func runQuery(t *testing.T, ds *Datasource, model string) backend.DataResponse {
	t.Helper()
	return runQueries(t, ds, rangeFrom, rangeTo, model).Responses["A"]
}

func frameOf(t *testing.T, r backend.DataResponse) *data.Frame {
	t.Helper()
	if r.Error != nil || len(r.Frames) != 1 {
		t.Fatalf("response error %v, %d frames", r.Error, len(r.Frames))
	}
	return r.Frames[0]
}

func friendlyIDs(f *data.Frame) []string {
	var out []string
	for i := 0; i < f.Fields[0].Len(); i++ {
		out = append(out, f.Fields[0].At(i).(string))
	}
	return out
}

func inRange(friendly string) string { return issueAt(friendly, rangeFrom.Add(24*time.Hour)) }

func TestQueryDefaults(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{inRange("A-1")}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{}`))
	if frame.RefID != "A" || !reflect.DeepEqual(friendlyIDs(frame), []string{"A-1"}) {
		t.Fatalf("refID %q ids %v", frame.RefID, friendlyIDs(frame))
	}
	want := url.Values{"state": {"unresolved"}, "sort": {"last_seen"}, "order": {"desc"}}
	if got := f.issueRequests(); len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
}

func TestQueryModel(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{inRange("A-1")}
	frameOf(t, runQuery(t, f.datasource(t),
		`{"queryType":"issues","project":1,"state":"muted","q":"x","sort":"digested_event_count","order":"asc"}`))
	want := url.Values{"project": {"1"}, "state": {"muted"}, "q": {"x"}, "sort": {"digested_event_count"}, "order": {"asc"}}
	if got := f.issueRequests(); len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("requests %v, want %v", got, want)
	}
}

func TestQueryRejectsUnknownValues(t *testing.T) {
	for _, key := range []string{"state", "sort", "order", "queryType"} {
		t.Run(key, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := runQuery(t, f.datasource(t), fmt.Sprintf(`{%q:"bogus"}`, key))
			want := fmt.Sprintf(`Unknown %s "bogus".`, key)
			if r.Error == nil || r.Error.Error() != want || r.Status != 400 || r.ErrorSource != backend.ErrorSourceDownstream {
				t.Fatalf("got %v status %d source %q, want %q 400", r.Error, r.Status, r.ErrorSource, want)
			}
			if n := len(f.issueRequests()); n != 0 {
				t.Fatalf("%d issue requests", n)
			}
		})
	}
}

func TestQueryTimeRange(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{strings.Join([]string{
		issueAt("AFTER", rangeTo.Add(time.Hour)),
		issueAt("IN-1", rangeTo.Add(-time.Hour)),
		issueAt("IN-2", rangeFrom.Add(time.Hour)),
		issueAt("BEFORE", rangeFrom.Add(-time.Hour)),
	}, ",")}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{}`))
	if got := friendlyIDs(frame); !reflect.DeepEqual(got, []string{"IN-1", "IN-2"}) {
		t.Fatalf("rows %v", got)
	}
}

func TestQueryPastRangeKeepsPaging(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{issueAt("NEW", rangeTo.Add(time.Hour)), inRange("IN")}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{}`))
	if got := friendlyIDs(frame); !reflect.DeepEqual(got, []string{"IN"}) {
		t.Fatalf("rows %v", got)
	}
	if n := len(f.issueRequests()); n != 2 {
		t.Fatalf("%d issue requests, want 2", n)
	}
}

func TestQueryCache(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{inRange("A-1")}
	ds := f.datasource(t)
	const base = `{"project":1,"state":"open","q":"x","sort":"last_seen","order":"desc"}`
	ask := func(from time.Time, model string) {
		t.Helper()
		frameOf(t, runQueries(t, ds, from, rangeTo, model).Responses["A"])
	}
	expect := func(n int, what string) {
		t.Helper()
		if got := len(f.issueRequests()); got != n {
			t.Fatalf("%s: %d issue requests, want %d", what, got, n)
		}
	}

	ask(rangeFrom, base)
	ask(rangeFrom, base)
	expect(1, "same query twice")
	ask(rangeFrom.Add(time.Hour), base)
	expect(1, "later from")
	ask(rangeFrom.Add(-time.Hour), base)
	expect(2, "earlier from")

	for i, variant := range []string{
		`{"project":2,"state":"open","q":"x","sort":"last_seen","order":"desc"}`,
		`{"project":1,"state":"muted","q":"x","sort":"last_seen","order":"desc"}`,
		`{"project":1,"state":"open","q":"y","sort":"last_seen","order":"desc"}`,
		`{"project":1,"state":"open","q":"x","sort":"digest_order","order":"desc"}`,
		`{"project":1,"state":"open","q":"x","sort":"last_seen","order":"asc"}`,
	} {
		ask(rangeFrom, variant)
		expect(3+i, "variant "+variant)
	}
}

func TestQueryTruncatedListIsNotCached(t *testing.T) {
	f := newFakeBugsink(t)
	f.endless = true
	ds := f.datasource(t)
	for i := 0; i < 2; i++ {
		frame := frameOf(t, runQuery(t, ds, `{}`))
		if len(frame.Meta.Notices) != 1 {
			t.Fatalf("notices %+v", frame.Meta.Notices)
		}
	}
	if n := len(f.issueRequests()); n != 40 {
		t.Fatalf("%d issue requests, want 40", n)
	}
}

func TestQueryProjectMustBeANumber(t *testing.T) {
	for _, model := range []string{`{"project":"1"}`, `{"project":"$project"}`, `{"project":true}`, `{"project":1.5}`} {
		t.Run(model, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := runQuery(t, f.datasource(t), model)
			if r.Error == nil || r.Error.Error() != "The project must be a number." || r.Status != 400 ||
				r.ErrorSource != backend.ErrorSourceDownstream {
				t.Fatalf("got %v status %d source %q", r.Error, r.Status, r.ErrorSource)
			}
			if n := len(f.issueRequests()); n != 0 {
				t.Fatalf("%d issue requests", n)
			}
		})
	}
}

func TestQueryProjectNullOrZeroMeansAllProjects(t *testing.T) {
	for _, model := range []string{`{"project":null}`, `{"project":0}`} {
		t.Run(model, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.pages = []string{inRange("A-1")}
			frameOf(t, runQuery(t, f.datasource(t), model))
			if got := f.issueRequests(); len(got) != 1 || got[0].Has("project") {
				t.Fatalf("requests %v", got)
			}
		})
	}
}

func TestQueryInvalidJSON(t *testing.T) {
	f := newFakeBugsink(t)
	r := runQuery(t, f.datasource(t), `{"project":`)
	if r.Error == nil || r.Error.Error() != "The query is not valid JSON." || r.Status != 400 {
		t.Fatalf("got %v status %d", r.Error, r.Status)
	}
}

func TestQueryErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		message string
		code    backend.Status
	}{
		{"capability", 403, `{"detail":"This token does not have the required capability: issues:read."}`,
			"The API token lacks issues:read.", 403},
		{"project forbidden", 403, `{"detail":"This token is not allowed to access this project."}`,
			"Bugsink refused the request: This token is not allowed to access this project.", 403},
		{"unauthorized", 401, `{"detail":"Invalid Bearer token."}`, "Bugsink rejected the API token.", 502},
		{"project missing", 404, `{"detail":"No Project matches the given query."}`,
			"Bugsink refused the request: No Project matches the given query.", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.issuesStatus, f.issuesBody = c.status, c.body
			r := runQuery(t, f.datasource(t), `{}`)
			if r.Error == nil || r.Error.Error() != c.message || r.Status != c.code || r.ErrorSource != backend.ErrorSourceDownstream {
				t.Fatalf("got %v status %d source %q", r.Error, r.Status, r.ErrorSource)
			}
			if strings.Contains(r.Error.Error(), secretToken) {
				t.Fatal("token in message")
			}
		})
	}

	t.Run("closed server", func(t *testing.T) {
		ds := newInstance(t, closedPortURL(t), secretToken, "")
		r := runQuery(t, ds, `{}`)
		if r.Error == nil || !strings.HasPrefix(r.Error.Error(), "Cannot reach Bugsink: ") || r.Status != 502 ||
			r.ErrorSource != backend.ErrorSourceDownstream || strings.Contains(r.Error.Error(), secretToken) {
			t.Fatalf("got %v status %d source %q", r.Error, r.Status, r.ErrorSource)
		}
	})
}

func TestQuerySettingsError(t *testing.T) {
	ds := newInstance(t, "", secretToken, "")
	resp := runQueries(t, ds, rangeFrom, rangeTo, `{}`, `{"state":"bogus"}`)
	for refID, r := range resp.Responses {
		if r.Error == nil || r.Error.Error() != msgEnterURL || r.Status != 400 || r.ErrorSource != backend.ErrorSourceDownstream {
			t.Fatalf("%s: got %v status %d source %q", refID, r.Error, r.Status, r.ErrorSource)
		}
	}
	if len(resp.Responses) != 2 {
		t.Fatalf("%d responses", len(resp.Responses))
	}
}

func TestQueryProjectsUnreadable(t *testing.T) {
	f := newFakeBugsink(t)
	f.projectsStatus = 403
	f.pages = []string{inRange("A-1")}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{}`))
	if got := frame.Fields[4].At(0); got != "1" {
		t.Fatalf("project %v, want \"1\"", got)
	}
}

func TestQueryProjectName(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{inRange("A-1")}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{}`))
	if got := frame.Fields[4].At(0); got != "alpha" {
		t.Fatalf("project %v, want alpha", got)
	}
}

func TestQueryTwoQueries(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{inRange("A-1")}
	resp := runQueries(t, f.datasource(t), rangeFrom, rangeTo, `{}`, `{"state":"bogus"}`)
	if got := friendlyIDs(frameOf(t, resp.Responses["A"])); !reflect.DeepEqual(got, []string{"A-1"}) {
		t.Fatalf("rows %v", got)
	}
	if r := resp.Responses["B"]; r.Error == nil || r.Error.Error() != `Unknown state "bogus".` {
		t.Fatalf("B error %v", r.Error)
	}
}

const capabilityDenied = `{"detail":"This token does not have the required capability: projects:read."}`

func TestProjectsFailureIsRemembered(t *testing.T) {
	f := newFakeBugsink(t)
	f.projectsStatus, f.errorBody = 403, capabilityDenied
	f.pages = []string{inRange("A-1")}
	ds := f.datasource(t)
	for i := 0; i < 2; i++ {
		frame := frameOf(t, runQuery(t, ds, `{}`))
		if got := frame.Fields[4].At(0); got != "1" {
			t.Fatalf("query %d: project %v, want \"1\"", i, got)
		}
	}
	r := callResource(t, ds, "GET", "projects")
	if r.status != 502 || messageOf(t, r) != "The API token lacks projects:read." {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	if n := f.projectRequests(); n != 1 {
		t.Fatalf("%d project requests, want 1", n)
	}
}

func TestProjectsFailureExpires(t *testing.T) {
	f := newFakeBugsink(t)
	f.projectsStatus, f.errorBody = 403, capabilityDenied
	ds := f.datasource(t)
	_, _ = ds.projects(context.Background())
	_, _ = ds.projects(context.Background())
	if n := f.projectRequests(); n != 1 {
		t.Fatalf("%d project requests before expiry, want 1", n)
	}
	ds.projectCache.now = func() time.Time { return time.Now().Add(cacheTTL + time.Second) }
	_, _ = ds.projects(context.Background())
	if n := f.projectRequests(); n != 2 {
		t.Fatalf("%d project requests, want 2", n)
	}
}

func TestProjectsCancelledReadIsNotRemembered(t *testing.T) {
	f := newFakeBugsink(t)
	f.projectsBody = `{"id":1,"name":"alpha"},{"id":2,"name":"beta"}`
	ds := f.datasource(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ds.projects(ctx); err == nil {
		t.Fatal("cancelled read succeeded")
	}
	projects, err := ds.projects(context.Background())
	if err != nil || len(projects) != 2 {
		t.Fatalf("projects %v, err %v", projects, err)
	}
}
