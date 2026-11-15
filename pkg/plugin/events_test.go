// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	defaultIssueUUID  = "0b1e1c9e-1a2b-4c3d-8e4f-0123456789ab"
	defaultEventID    = "0f0e0d0c-0b0a-4908-8706-050403020100"
	defaultEventSDKID = "426ac644-fafb-475d-9050-46d654730f1b"
)

var olderThanRange = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

func eventAt(id, eventID, issue string, ts time.Time) string {
	return fmt.Sprintf(`{"id":%q,"ingested_at":"2020-01-01T00:00:01Z","digested_at":"2020-01-01T00:00:02Z",`+
		`"issue":%q,"grouping":1,"event_id":%q,"project":1,"timestamp":%q,"digest_order":5}`,
		id, issue, eventID, ts.Format(time.RFC3339))
}

// uuidN is a valid uuid that differs by n.
func uuidN(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func issueRow(id, friendly string) string {
	return strings.Replace(issueAt(friendly, rangeTo), defaultIssueUUID, id, 1)
}

func stringColumn(t *testing.T, f *data.Frame, name string) []string {
	t.Helper()
	for _, fld := range f.Fields {
		if fld.Name == name {
			var out []string
			for i := 0; i < fld.Len(); i++ {
				out = append(out, fld.At(i).(string))
			}
			return out
		}
	}
	t.Fatalf("no field %q", name)
	return nil
}

func notices(f *data.Frame) []string {
	var out []string
	for _, n := range f.Meta.Notices {
		if n.Severity != data.NoticeSeverityWarning {
			return []string{"not a warning: " + n.Text}
		}
		out = append(out, n.Text)
	}
	return out
}

func TestEventsQueryValidation(t *testing.T) {
	cases := []struct{ name, model, message string }{
		{"issue", `{"queryType":"events","issue":"../projects"}`, `"../projects" is not an issue id.`},
		{"event id", `{"queryType":"events","eventId":"nope","project":1}`, "The event ID must be a UUID."},
		{"event id without project", `{"queryType":"events","eventId":"426ac644fafb475d905046d654730f1b"}`,
			"Looking up an event ID needs a project."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := runQuery(t, f.datasource(t), c.model)
			if r.Error == nil || r.Error.Error() != c.message || r.Status != 400 || r.ErrorSource != backend.ErrorSourceDownstream {
				t.Fatalf("got %v status %d source %q, want %q 400", r.Error, r.Status, r.ErrorSource, c.message)
			}
			if n := f.requestCount(); n != 0 {
				t.Fatalf("%d Bugsink requests", n)
			}
		})
	}
}

func TestEventsQueryNamingNothing(t *testing.T) {
	const message = "An events query needs an issue, an event ID or trace:<id> in Search."
	for _, model := range []string{
		`{"queryType":"events","q":"failure"}`,
		`{"queryType":"events","q":"trace:"}`,
		`{"queryType":"events","q":"notrace:abc"}`,
	} {
		t.Run(model, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := runQuery(t, f.datasource(t), model)
			if r.Error != nil {
				t.Fatalf("got %v, want no error", r.Error)
			}
			frame := frameOf(t, r)
			if frame.Name != "events" || len(frame.Fields) != 8 || frame.Rows() != 0 {
				t.Fatalf("frame %q with %d fields and %d rows", frame.Name, len(frame.Fields), frame.Rows())
			}
			if len(frame.Meta.Notices) != 1 || frame.Meta.Notices[0].Severity != data.NoticeSeverityInfo || frame.Meta.Notices[0].Text != message {
				t.Fatalf("notices %v", frame.Meta.Notices)
			}
			if n := f.requestCount(); n != 0 {
				t.Fatalf("%d Bugsink requests", n)
			}
		})
	}
}

func TestEventsOfIssue(t *testing.T) {
	f := newFakeBugsink(t)
	f.eventPages = map[string][]string{defaultIssueUUID: {
		eventAt(uuidN(1), uuidN(11), defaultIssueUUID, olderThanRange) + "," + eventAt(uuidN(2), uuidN(12), defaultIssueUUID, olderThanRange),
	}}
	frame := frameOf(t, runQuery(t, f.datasource(t),
		`{"queryType":"events","issue":"SAMPLE-PYTHON-2","q":"environment:development","state":"muted","sort":"digested_event_count","order":"asc"}`))

	if got := stringColumn(t, frame, "id"); !reflect.DeepEqual(got, []string{uuidN(1), uuidN(2)}) {
		t.Fatalf("event ids %v", got)
	}
	if got := stringColumn(t, frame, "issue"); !reflect.DeepEqual(got, []string{"SAMPLE-PYTHON-2", "SAMPLE-PYTHON-2"}) {
		t.Fatalf("issues %v", got)
	}
	if n := f.pathCount("/api/canonical/0/issues/SAMPLE-PYTHON-2/"); n != 1 {
		t.Fatalf("%d issue reads", n)
	}
	want := url.Values{"issue": {defaultIssueUUID}, "q": {"environment:development"}, "order": {"desc"}}
	if got := f.eventRequestList(); len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("event requests %v, want %v", got, want)
	}
}

func TestEventsOfIssueBound(t *testing.T) {
	f := newFakeBugsink(t)
	f.eventsEndless = true
	frame := frameOf(t, runQuery(t, f.datasource(t), `{"queryType":"events","issue":"SAMPLE-PYTHON-2"}`))
	if n := len(f.eventRequestList()); n != 4 {
		t.Fatalf("%d event requests, want 4", n)
	}
	if got := notices(frame); !reflect.DeepEqual(got, []string{"Only the newest 1000 events were read."}) {
		t.Fatalf("notices %v", got)
	}
}

func TestEventLookup(t *testing.T) {
	f := newFakeBugsink(t)
	frame := frameOf(t, runQuery(t, f.datasource(t), `{"queryType":"events","eventId":"426ac644fafb475d905046d654730f1b","project":1}`))
	want := url.Values{"project": {"1"}, "event_id": {"426ac644-fafb-475d-9050-46d654730f1b"}}
	if len(f.lookupRequests) != 1 || !reflect.DeepEqual(f.lookupRequests[0], want) {
		t.Fatalf("lookup requests %v, want %v", f.lookupRequests, want)
	}
	if n := f.pathCount("/api/canonical/0/issues/" + defaultIssueUUID + "/"); n != 1 {
		t.Fatalf("%d issue reads", n)
	}
	if got := stringColumn(t, frame, "event_id"); !reflect.DeepEqual(got, []string{defaultEventSDKID}) {
		t.Fatalf("event ids %v", got)
	}
	if got := stringColumn(t, frame, "issue"); !reflect.DeepEqual(got, []string{"SAMPLE-PYTHON-2"}) {
		t.Fatalf("issues %v", got)
	}
}

func TestEventLookupNotFound(t *testing.T) {
	f := newFakeBugsink(t)
	f.lookupStatus, f.lookupBody = 404, `{"detail":"No Event matches the given query."}`
	r := runQuery(t, f.datasource(t), `{"queryType":"events","eventId":"426ac644fafb475d905046d654730f1b","project":1}`)
	want := "Bugsink refused the request: No Event matches the given query."
	if r.Error == nil || r.Error.Error() != want || r.Status != 404 {
		t.Fatalf("got %v status %d, want %q 404", r.Error, r.Status, want)
	}
}

func traceIssues(n int) (ids []string, page string) {
	var rows []string
	for i := 1; i <= n; i++ {
		ids = append(ids, uuidN(i))
		rows = append(rows, issueRow(uuidN(i), fmt.Sprintf("T-%d", i)))
	}
	return ids, strings.Join(rows, ",")
}

func TestEventsOfTrace(t *testing.T) {
	f := newFakeBugsink(t)
	ids, page := traceIssues(2)
	f.pages = []string{page}
	f.eventPages = map[string][]string{
		ids[0]: {eventAt(uuidN(101), uuidN(201), ids[0], olderThanRange) + "," + eventAt(uuidN(102), uuidN(202), ids[0], olderThanRange)},
		ids[1]: {eventAt(uuidN(103), uuidN(203), ids[1], olderThanRange)},
	}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{"queryType":"events","q":"trace:abc"}`))

	if got := stringColumn(t, frame, "id"); !reflect.DeepEqual(got, []string{uuidN(101), uuidN(102), uuidN(103)}) {
		t.Fatalf("event ids %v", got)
	}
	if got := stringColumn(t, frame, "issue"); !reflect.DeepEqual(got, []string{"T-1", "T-1", "T-2"}) {
		t.Fatalf("issues %v", got)
	}
	wantIssues := url.Values{"q": {"trace:abc"}, "state": {"all"}, "sort": {"last_seen"}, "order": {"desc"}}
	if got := f.issueRequests(); len(got) != 1 || !reflect.DeepEqual(got[0], wantIssues) {
		t.Fatalf("issue requests %v, want %v", got, wantIssues)
	}
	got := f.eventRequestList()
	if len(got) != 2 {
		t.Fatalf("%d event requests", len(got))
	}
	for i, id := range ids {
		want := url.Values{"issue": {id}, "q": {"trace:abc"}, "order": {"desc"}}
		if !reflect.DeepEqual(got[i], want) {
			t.Errorf("event request %d: %v, want %v", i, got[i], want)
		}
	}
	if n := notices(frame); len(n) != 0 {
		t.Fatalf("notices %v", n)
	}
}

func TestEventsOfTraceBounds(t *testing.T) {
	f := newFakeBugsink(t)
	ids, page := traceIssues(12)
	f.pages = []string{page}
	f.eventPages = map[string][]string{
		ids[0]: {eventAt(uuidN(101), uuidN(201), ids[0], olderThanRange), eventAt(uuidN(102), uuidN(202), ids[0], olderThanRange)},
	}
	frame := frameOf(t, runQuery(t, f.datasource(t), `{"queryType":"events","q":"trace:abc"}`))

	requests := f.eventRequestList()
	if len(requests) != 10 {
		t.Fatalf("%d event requests, want one per issue and no second page: %v", len(requests), requests)
	}
	for _, r := range requests {
		if r.Has("cursor") || r.Get("issue") == ids[10] || r.Get("issue") == ids[11] {
			t.Fatalf("unexpected request %v", r)
		}
	}
	want := []string{
		"This trace has events in more than 10 issues. Only the 10 last seen were read.",
		"Only the newest 250 events of T-1 in this trace were read.",
	}
	if got := notices(frame); !reflect.DeepEqual(got, want) {
		t.Fatalf("notices %v, want %v", got, want)
	}
}

func TestEventsOfTraceWithProject(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{""}
	ds := f.datasource(t)
	frameOf(t, runQuery(t, ds, `{"queryType":"events","q":"trace:abc","project":2}`))
	if got := f.issueRequests(); len(got) != 1 || got[0].Get("project") != "2" {
		t.Fatalf("issue requests %v", got)
	}

	f.issuesStatus, f.issuesBody = 403, `{"detail":"This token is not allowed to access this project."}`
	r := runQuery(t, ds, `{"queryType":"events","q":"trace:other","project":2}`)
	want := "Bugsink refused the request: This token is not allowed to access this project."
	if r.Error == nil || r.Error.Error() != want || r.Status != 403 {
		t.Fatalf("got %v status %d, want %q 403", r.Error, r.Status, want)
	}
}

func TestEventsCache(t *testing.T) {
	f := newFakeBugsink(t)
	f.eventPages = map[string][]string{defaultIssueUUID: {eventAt(uuidN(1), uuidN(11), defaultIssueUUID, olderThanRange)}}
	ds := f.datasource(t)
	const model = `{"queryType":"events","issue":"SAMPLE-PYTHON-2","q":"a"}`

	frameOf(t, runQuery(t, ds, model))
	after := f.requestCount()
	frameOf(t, runQuery(t, ds, model))
	if n := f.requestCount(); n != after {
		t.Fatalf("second run made %d requests", n-after)
	}
	frameOf(t, runQuery(t, ds, `{"queryType":"events","issue":"SAMPLE-PYTHON-2","q":"b"}`))
	if n := f.requestCount(); n == after {
		t.Fatal("a different q made no request")
	}

	f.setEventsStatus(500)
	const failing = `{"queryType":"events","issue":"SAMPLE-PYTHON-2","q":"c"}`
	if r := runQuery(t, ds, failing); r.Error == nil {
		t.Fatal("expected an error")
	}
	f.setEventsStatus(0)
	before := f.requestCount()
	frame := frameOf(t, runQuery(t, ds, failing))
	if f.requestCount() == before {
		t.Fatal("the failed query was served from the cache")
	}
	if got := stringColumn(t, frame, "id"); !reflect.DeepEqual(got, []string{uuidN(1)}) {
		t.Fatalf("event ids %v", got)
	}
}

func TestEventsOfTraceKeepsWholeQuery(t *testing.T) {
	f := newFakeBugsink(t)
	ids, page := traceIssues(2)
	f.pages = []string{page}
	f.eventPages = map[string][]string{ids[0]: {eventAt(uuidN(101), uuidN(201), ids[0], olderThanRange)}}
	frameOf(t, runQuery(t, f.datasource(t), `{"queryType":"events","q":"environment:dev trace:abc"}`))

	if got := f.issueRequests(); len(got) != 1 || got[0].Get("q") != "environment:dev trace:abc" {
		t.Fatalf("issue requests %v", got)
	}
	got := f.eventRequestList()
	if len(got) != 2 {
		t.Fatalf("%d event requests", len(got))
	}
	for _, r := range got {
		if r.Get("q") != "environment:dev trace:abc" {
			t.Errorf("event request %v", r)
		}
	}
}

func TestEventsOfTraceFailingEventList(t *testing.T) {
	f := newFakeBugsink(t)
	_, page := traceIssues(2)
	f.pages = []string{page}
	f.setEventsStatus(500)
	if r := runQuery(t, f.datasource(t), `{"queryType":"events","q":"trace:abc"}`); r.Error == nil {
		t.Fatal("expected an error")
	}
}

func TestTwoQueryTypes(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{inRange("A-1")}
	f.eventPages = map[string][]string{defaultIssueUUID: {eventAt(uuidN(1), uuidN(11), defaultIssueUUID, olderThanRange)}}
	resp := runQueries(t, f.datasource(t), rangeFrom, rangeTo, `{"queryType":"issues"}`, `{"queryType":"events","issue":"SAMPLE-PYTHON-2"}`)
	a, b := frameOf(t, resp.Responses["A"]), frameOf(t, resp.Responses["B"])
	if a.Name != "issues" || b.Name != "events" || a.RefID != "A" || b.RefID != "B" {
		t.Fatalf("frames %q/%q refs %q/%q", a.Name, b.Name, a.RefID, b.RefID)
	}
}
