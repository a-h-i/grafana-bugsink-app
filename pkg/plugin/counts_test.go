// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var countsNow = utc("2026-11-15T13:48:00Z")

const issueCapMessage = "More than 5000 issues match, so they cannot be counted. Narrow the query with a project, a state or a search."

// runCount runs one query the way Grafana's alerting does: no user and a zero time range.
func runCount(t *testing.T, ds *Datasource, model string) backend.DataResponse {
	t.Helper()
	resp, err := ds.QueryData(context.Background(), alertRequest(model))
	if err != nil {
		t.Fatalf("QueryData returned error: %v", err)
	}
	return resp.Responses["A"]
}

// alertRequest is a query request the way Grafana's alerting sends it.
func alertRequest(model string) *backend.QueryDataRequest {
	return &backend.QueryDataRequest{
		PluginContext: backend.PluginContext{},
		Headers:       map[string]string{"FromAlert": "true"},
		Queries:       []backend.DataQuery{{RefID: "A", JSON: json.RawMessage(model)}},
	}
}

// countResult is the outcome of a count query run on another goroutine.
type countResult struct {
	resp *backend.QueryDataResponse
	err  error
}

// startCount runs a count query on a new goroutine and delivers its outcome on the returned channel.
func startCount(ds *Datasource, model string) <-chan countResult {
	out := make(chan countResult, 1)
	go func() {
		resp, err := ds.QueryData(context.Background(), alertRequest(model))
		out <- countResult{resp, err}
	}()
	return out
}

// awaitCount receives a count query's outcome and fails the test when it is an error.
func awaitCount(t *testing.T, results <-chan countResult) backend.DataResponse {
	t.Helper()
	r := <-results
	if r.err != nil {
		t.Fatalf("QueryData returned error: %v", r.err)
	}
	return r.resp.Responses["A"]
}

func countIssuesAt(n int, lastSeen time.Time) string {
	items := make([]string, n)
	for i := range items {
		items[i] = issueAt(fmt.Sprintf("X-%d", i+1), lastSeen)
	}
	return strings.Join(items, ",")
}

func issueCountDatasource(t *testing.T, f *fakeBugsink) *Datasource {
	ds := f.datasource(t)
	ds.now = func() time.Time { return countsNow }
	return ds
}

// valueOf checks the numeric frame of the contract and returns its one value.
func valueOf(t *testing.T, r backend.DataResponse, name string, labels data.Labels) int64 {
	t.Helper()
	frame := frameOf(t, r)
	if frame.Name != name || frame.RefID != "A" {
		t.Fatalf("frame name %q ref %q", frame.Name, frame.RefID)
	}
	if frame.Meta == nil || frame.Meta.Type != data.FrameTypeNumericMulti || frame.Meta.TypeVersion != (data.FrameTypeVersion{0, 1}) {
		t.Fatalf("meta %+v", frame.Meta)
	}
	if len(frame.Fields) != 1 || frame.Fields[0].Name != "count" || frame.Fields[0].Type() != data.FieldTypeInt64 || frame.Fields[0].Len() != 1 {
		t.Fatalf("fields %+v", frame.Fields)
	}
	field := frame.Fields[0]
	if !reflect.DeepEqual(field.Labels, labels) {
		t.Fatalf("labels %v, want %v", field.Labels, labels)
	}
	if field.Config == nil || field.Config.DisplayNameFromDS != name {
		t.Fatalf("config %+v", field.Config)
	}
	return field.At(0).(int64)
}

func TestIssueCountProject(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{countIssuesAt(200, countsNow.Add(-time.Hour)), countIssuesAt(51, countsNow.Add(-2*time.Hour))}
	ds := issueCountDatasource(t, f)
	r := runQueries(t, ds, countsNow.Add(-5*time.Minute), countsNow,
		`{"queryType":"issueCount","project":1,"state":"open"}`).Responses["A"]
	if got := valueOf(t, r, "Issues", data.Labels{"project_id": "1"}); got != 251 {
		t.Fatalf("count %d, want 251", got)
	}
	reqs := f.issueRequests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	for _, key := range [][2]string{{"project", "1"}, {"state", "open"}, {"sort", "last_seen"}, {"order", "desc"}} {
		if reqs[0].Get(key[0]) != key[1] {
			t.Fatalf("request %v lacks %s=%s", reqs[0], key[0], key[1])
		}
	}
}

func TestIssueCountNoMatchIsZero(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{""}
	r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount"}`)
	if got := valueOf(t, r, "Issues", nil); got != 0 {
		t.Fatalf("count %d, want 0", got)
	}
}

func TestIssueCountSeenInLastMinutes(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{
		issueAt("X-1", countsNow.Add(-time.Minute)) + "," + issueAt("X-2", countsNow.Add(-4*time.Minute)) + "," +
			issueAt("X-3", countsNow.Add(-6*time.Minute)),
		issueAt("X-4", countsNow.Add(-7*time.Minute)),
	}
	r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount","minutes":5,"state":"all"}`)
	if got := valueOf(t, r, "Issues", nil); got != 2 {
		t.Fatalf("count %d, want 2", got)
	}
	if n := len(f.issueRequests()); n != 1 {
		t.Fatalf("%d list requests, want 1", n)
	}
}

func TestIssueCountCap(t *testing.T) {
	f := newFakeBugsink(t)
	f.endless = true
	ds := issueCountDatasource(t, f)
	for round := 1; round <= 2; round++ {
		r := runCount(t, ds, `{"queryType":"issueCount","state":"all"}`)
		if r.Error == nil || r.Error.Error() != issueCapMessage || r.Status != backend.StatusBadRequest || len(r.Frames) != 0 {
			t.Fatalf("round %d: %v status %d frames %d", round, r.Error, r.Status, len(r.Frames))
		}
		if n := len(f.issueRequests()); n != 20*round {
			t.Fatalf("round %d: %d requests, want %d", round, n, 20*round)
		}
	}
}

func TestIssueCountAtTheCapAndOnePast(t *testing.T) {
	pages := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = issueAt(fmt.Sprintf("X-%d", i+1), countsNow.Add(-time.Hour))
		}
		return out
	}
	t.Run("a list ending on page 20 is counted", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.pages = pages(20)
		r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount","state":"all"}`)
		if got := valueOf(t, r, "Issues", nil); got != 20 {
			t.Fatalf("count %d, want 20", got)
		}
		if n := len(f.issueRequests()); n != 20 {
			t.Fatalf("%d requests, want 20", n)
		}
	})
	t.Run("a list with a 21st page is the cap error", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.pages = pages(21)
		r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount","state":"all"}`)
		if r.Error == nil || r.Error.Error() != issueCapMessage {
			t.Fatalf("got %v", r.Error)
		}
		if n := len(f.issueRequests()); n != 20 {
			t.Fatalf("%d requests, want 20", n)
		}
	})
}

func TestIssueCountValidation(t *testing.T) {
	const minutesMessage = "The minutes must be a whole number from 1 to 1440."
	for _, raw := range []string{`0`, `1441`, `2.5`, `"5"`, `-1`, `true`, `1e2`, `[]`} {
		t.Run("minutes "+raw, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount","minutes":`+raw+`}`)
			if r.Error == nil || r.Error.Error() != minutesMessage || r.Status != backend.StatusBadRequest {
				t.Fatalf("got %v status %d", r.Error, r.Status)
			}
			if f.requestCount() != 0 {
				t.Fatal("Bugsink was asked")
			}
		})
	}
	for _, model := range []string{`{"queryType":"issueCount"}`, `{"queryType":"issueCount","minutes":null}`,
		`{"queryType":"issueCount","minutes":1}`, `{"queryType":"issueCount","minutes":1440}`} {
		f := newFakeBugsink(t)
		f.pages = []string{issueAt("X-1", countsNow.Add(-time.Minute))}
		if r := runCount(t, issueCountDatasource(t, f), model); r.Error != nil {
			t.Fatalf("%s: %v", model, r.Error)
		}
	}
	t.Run("any time counts old issues", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.pages = []string{issueAt("X-1", countsNow.Add(-1000*time.Hour))}
		r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount","minutes":null}`)
		if got := valueOf(t, r, "Issues", nil); got != 1 {
			t.Fatalf("count %d, want 1", got)
		}
	})
	t.Run("state", func(t *testing.T) {
		f := newFakeBugsink(t)
		r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount","state":"bogus"}`)
		if r.Error == nil || r.Error.Error() != `Unknown state "bogus".` || f.requestCount() != 0 {
			t.Fatalf("got %v, %d requests", r.Error, f.requestCount())
		}
	})
}

func TestIssueCountSharesTheIssueCache(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{issueAt("X-1", countsNow.Add(-2*time.Hour)) + "," + issueAt("X-2", countsNow.Add(-10*time.Minute))}
	ds := issueCountDatasource(t, f)
	day := runQueries(t, ds, countsNow.Add(-24*time.Hour), countsNow, `{"queryType":"issues","project":1,"state":"open"}`)
	if day.Responses["A"].Error != nil {
		t.Fatal(day.Responses["A"].Error)
	}
	r := runCount(t, ds, `{"queryType":"issueCount","project":1,"state":"open","minutes":60}`)
	if got := valueOf(t, r, "Issues", data.Labels{"project_id": "1"}); got != 1 {
		t.Fatalf("count %d, want 1", got)
	}
	if n := len(f.issueRequests()); n != 1 {
		t.Fatalf("%d list reads, want 1", n)
	}
}

func TestIssueReadAcrossAClearIsNotShared(t *testing.T) {
	const model = `{"queryType":"issueCount"}`
	release := make(chan struct{})
	var releaseOnce sync.Once
	var mu sync.Mutex
	reads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reads++
		n := reads
		mu.Unlock()
		issues := countIssuesAt(2, countsNow)
		if n == 1 {
			<-release
			issues = countIssuesAt(1, countsNow)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"next":null,"previous":null,"results":[%s]}`, issues)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	ds := newInstance(t, srv.URL, secretToken, "")
	ds.now = func() time.Time { return countsNow }
	readCount := func() int { mu.Lock(); defer mu.Unlock(); return reads }

	first := startCount(ds, model)
	eventually(t, "the first list read held", func() bool { return readCount() == 1 })
	ds.issueCache.clear()
	second := startCount(ds, model)
	eventually(t, "a second list read after the clear", func() bool { return readCount() == 2 })
	if got := valueOf(t, awaitCount(t, second), "Issues", nil); got != 2 {
		t.Fatalf("second query count %d, want the answer read after the clear, 2", got)
	}
	releaseOnce.Do(func() { close(release) })
	if got := valueOf(t, awaitCount(t, first), "Issues", nil); got != 1 {
		t.Fatalf("first query count %d, want 1", got)
	}
}

func TestIssueCountFailureIsNotCached(t *testing.T) {
	f := newFakeBugsink(t)
	f.issuesStatus, f.issuesBody = 500, `{"detail":"boom"}`
	ds := issueCountDatasource(t, f)
	if r := runCount(t, ds, `{"queryType":"issueCount"}`); r.Error == nil {
		t.Fatal("no error")
	}
	f.mu.Lock()
	f.issuesStatus = 0
	f.pages = []string{issueAt("X-1", countsNow)}
	f.mu.Unlock()
	r := runCount(t, ds, `{"queryType":"issueCount"}`)
	if got := valueOf(t, r, "Issues", nil); got != 1 {
		t.Fatalf("count %d, want 1", got)
	}
	if n := len(f.issueRequests()); n != 2 {
		t.Fatalf("%d requests, want 2", n)
	}
}

// fakeHourCounts serves event-counts reads with 24 hourly buckets ending at last, and records them.
type fakeHourCounts struct {
	mu     sync.Mutex
	last   time.Time
	counts map[time.Time]int // by hour, 1 where absent
	status int
	body   string
	seen   []seenRequest
	srv    *httptest.Server
}

func newFakeHourCounts(t *testing.T, last string) *fakeHourCounts {
	t.Helper()
	f := &fakeHourCounts{last: utc(last), counts: map[time.Time]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeHourCounts) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, seenRequest{path: r.URL.Path, rawQuery: r.URL.RawQuery})
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = fmt.Fprint(w, f.body)
		return
	}
	if f.body != "" {
		_, _ = fmt.Fprint(w, f.body)
		return
	}
	var buckets []string
	for i := 23; i >= 0; i-- {
		h := f.last.Add(-time.Duration(i) * time.Hour)
		n, ok := f.counts[h]
		if !ok {
			n = 1
		}
		buckets = append(buckets, fmt.Sprintf(`{"timestamp":%q,"count":%d}`, h.Format(time.RFC3339), n))
	}
	_, _ = fmt.Fprintf(w, `{"start":%q,"end":%q,"interval_seconds":3600,"buckets":[%s]}`,
		f.last.Add(-23*time.Hour).Format(time.RFC3339), f.last.Add(time.Hour).Format(time.RFC3339), strings.Join(buckets, ","))
}

func (f *fakeHourCounts) requests() []seenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seenRequest(nil), f.seen...)
}

func (f *fakeHourCounts) datasource(t *testing.T, now *time.Time) *Datasource {
	ds := newInstance(t, f.srv.URL, secretToken, `{"browserUrl":"http://b"}`)
	ds.now = func() time.Time { return *now }
	return ds
}

func TestEventCountProject(t *testing.T) {
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	f.counts[utc("2026-11-15T13:00:00Z")] = 7
	f.counts[utc("2026-11-15T12:00:00Z")] = 5
	now := countsNow
	ds := f.datasource(t, &now)
	r := runCount(t, ds, `{"queryType":"eventCount","project":1}`)
	if got := valueOf(t, r, "Events", data.Labels{"project_id": "1"}); got != 7 {
		t.Fatalf("count %d, want 7", got)
	}
	reqs := f.requests()
	if len(reqs) != 1 || reqs[0].path != "/api/canonical/0/projects/1/event-counts/" || reqs[0].rawQuery != "" {
		t.Fatalf("requests %+v", reqs)
	}
	r = runCount(t, ds, `{"queryType":"eventCount","project":1,"hours":2}`)
	if got := valueOf(t, r, "Events", data.Labels{"project_id": "1"}); got != 12 {
		t.Fatalf("hours 2: count %d, want 12", got)
	}
}

func TestEventCountNoEventsIsZero(t *testing.T) {
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	f.counts[utc("2026-11-15T13:00:00Z")] = 0
	now := countsNow
	r := runCount(t, f.datasource(t, &now), `{"queryType":"eventCount","project":1}`)
	if got := valueOf(t, r, "Events", data.Labels{"project_id": "1"}); got != 0 {
		t.Fatalf("count %d, want 0", got)
	}
}

func TestEventCountIssueWins(t *testing.T) {
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	now := countsNow
	r := runCount(t, f.datasource(t, &now), `{"queryType":"eventCount","issue":"X-1","project":1}`)
	if got := valueOf(t, r, "Events", data.Labels{"issue": "X-1"}); got != 1 {
		t.Fatalf("count %d, want 1", got)
	}
	reqs := f.requests()
	if len(reqs) != 1 || reqs[0].path != "/api/canonical/0/issues/X-1/event-counts/" {
		t.Fatalf("requests %+v", reqs)
	}
}

func TestEventCountTopOfHour(t *testing.T) {
	for _, c := range []struct {
		name, last string
		want       int64
	}{
		{"bugsink behind", "2026-11-15T12:00:00Z", 12},
		{"bugsink ahead", "2026-11-15T14:00:00Z", 14},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeHourCounts(t, c.last)
			f.counts[utc(c.last)] = int(c.want)
			now := countsNow
			ds := f.datasource(t, &now)
			for round := 1; round <= 2; round++ {
				r := runCount(t, ds, `{"queryType":"eventCount","project":1}`)
				if got := valueOf(t, r, "Events", data.Labels{"project_id": "1"}); got != c.want {
					t.Fatalf("round %d: count %d, want %d", round, got, c.want)
				}
				if n := len(f.requests()); n != round {
					t.Fatalf("round %d: %d requests", round, n)
				}
			}
		})
	}
}

func TestEventCountCache(t *testing.T) {
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	now := countsNow
	ds := f.datasource(t, &now)
	for range 2 {
		runCount(t, ds, `{"queryType":"eventCount","project":1}`)
	}
	if n := len(f.requests()); n != 1 {
		t.Fatalf("%d requests within the minute, want 1", n)
	}
	now = utc("2026-11-15T13:59:30Z")
	runCount(t, ds, `{"queryType":"eventCount","project":1}`)
	if n := len(f.requests()); n != 1 {
		t.Fatalf("%d requests, want 1 while the cache is fresh", n)
	}
	f.mu.Lock()
	f.last = utc("2026-11-15T14:00:00Z")
	f.counts[f.last] = 3
	f.mu.Unlock()
	now = utc("2026-11-15T14:00:05Z")
	r := runCount(t, ds, `{"queryType":"eventCount","project":1}`)
	if got := valueOf(t, r, "Events", data.Labels{"project_id": "1"}); got != 3 {
		t.Fatalf("count %d after the hour turned, want 3", got)
	}
	if n := len(f.requests()); n != 2 {
		t.Fatalf("%d requests after the hour turned, want 2", n)
	}
}

func TestEventCountValidation(t *testing.T) {
	const hoursMessage = "The hours must be a whole number from 1 to 24."
	for _, raw := range []string{`0`, `25`, `1.5`, `"2"`, `-1`, `false`} {
		t.Run("hours "+raw, func(t *testing.T) {
			f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
			now := countsNow
			r := runCount(t, f.datasource(t, &now), `{"queryType":"eventCount","project":1,"hours":`+raw+`}`)
			if r.Error == nil || r.Error.Error() != hoursMessage || r.Status != backend.StatusBadRequest || len(f.requests()) != 0 {
				t.Fatalf("got %v status %d, %d requests", r.Error, r.Status, len(f.requests()))
			}
		})
	}
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	now := countsNow
	ds := f.datasource(t, &now)
	for _, model := range []string{`{"queryType":"eventCount","project":1,"hours":null}`,
		`{"queryType":"eventCount","project":1,"hours":24}`} {
		if r := runCount(t, ds, model); r.Error != nil {
			t.Fatalf("%s: %v", model, r.Error)
		}
	}
	before := len(f.requests())
	r := runCount(t, ds, `{"queryType":"eventCount","issue":"a/b"}`)
	if r.Error == nil || r.Error.Error() != `"a/b" is not an issue id.` || r.Status != backend.StatusBadRequest {
		t.Fatalf("got %v status %d", r.Error, r.Status)
	}
	r = runCount(t, ds, `{"queryType":"eventCount"}`)
	frame := frameOf(t, r)
	if frame.Name != "Events" || len(frame.Fields) != 1 || frame.Fields[0].Len() != 0 || frame.Fields[0].Type() != data.FieldTypeInt64 {
		t.Fatalf("frame %+v", frame)
	}
	if frame.Meta == nil || len(frame.Meta.Notices) != 1 || frame.Meta.Notices[0].Severity != data.NoticeSeverityInfo ||
		frame.Meta.Notices[0].Text != "An event count needs a project or an issue." {
		t.Fatalf("notices %+v", frame.Meta)
	}
	if len(f.requests()) != before {
		t.Fatal("Bugsink was asked")
	}
}

func TestCountQueriesNeverAnswerZeroOnFailure(t *testing.T) {
	models := []string{`{"queryType":"issueCount","project":1}`, `{"queryType":"eventCount","project":1}`}
	for _, model := range models {
		t.Run("closed server "+model, func(t *testing.T) {
			ds := newInstance(t, closedPortURL(t), secretToken, "")
			r := runCount(t, ds, model)
			if r.Error == nil || !strings.HasPrefix(r.Error.Error(), "Cannot reach Bugsink: ") || r.Status != backend.StatusBadGateway ||
				r.ErrorSource != backend.ErrorSourceDownstream || len(r.Frames) != 0 || strings.Contains(r.Error.Error(), secretToken) {
				t.Fatalf("got %v status %d frames %d", r.Error, r.Status, len(r.Frames))
			}
		})
		t.Run("refused "+model, func(t *testing.T) {
			const detail = `{"detail":"This token is not allowed to access this project."}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, detail)
			}))
			defer srv.Close()
			ds := newInstance(t, srv.URL, secretToken, "")
			r := runCount(t, ds, model)
			if r.Error == nil || r.Error.Error() != "Bugsink refused the request: This token is not allowed to access this project." ||
				r.Status != backend.StatusForbidden || len(r.Frames) != 0 {
				t.Fatalf("got %v status %d frames %d", r.Error, r.Status, len(r.Frames))
			}
		})
	}
}

func TestEventCountMalformedAnswerIsAnError(t *testing.T) {
	for name, body := range map[string]string{
		"not json":       `<html>hi</html>`,
		"not hourly":     `{"interval_seconds":60,"buckets":[]}`,
		"no buckets":     `{"interval_seconds":3600}`,
		"negative":       `{"interval_seconds":3600,"buckets":[{"timestamp":"2026-11-15T13:00:00Z","count":-4}]}`,
		"off the hour":   `{"interval_seconds":3600,"buckets":[{"timestamp":"2026-11-15T13:30:00Z","count":4}]}`,
		"empty answer":   ``,
		"object buckets": `{"interval_seconds":3600,"buckets":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
			f.body = body
			if body == "" {
				f.body = " "
			}
			now := countsNow
			r := runCount(t, f.datasource(t, &now), `{"queryType":"eventCount","project":1}`)
			if r.Error == nil || len(r.Frames) != 0 {
				t.Fatalf("got %v, %d frames", r.Error, len(r.Frames))
			}
		})
	}
}

func TestEventCountShortAnswerIsAnError(t *testing.T) {
	for _, c := range []struct{ name, body, model string }{
		{"no buckets", `{"interval_seconds":3600,"buckets":[]}`, `{"queryType":"eventCount","project":1}`},
		{"fewer buckets than hours", `{"interval_seconds":3600,"buckets":[{"timestamp":"2026-11-15T13:00:00Z","count":4}]}`,
			`{"queryType":"eventCount","project":1,"hours":2}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
			f.body = c.body
			now := countsNow
			r := runCount(t, f.datasource(t, &now), c.model)
			if r.Error == nil || r.Error.Error() != "The answer did not look like a Bugsink API." ||
				r.Status != backend.StatusBadGateway || len(r.Frames) != 0 {
				t.Fatalf("got %v status %d, %d frames", r.Error, r.Status, len(r.Frames))
			}
		})
	}
}

func TestIssueCountMalformedAnswerIsAnError(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{`"not an issue"`}
	r := runCount(t, issueCountDatasource(t, f), `{"queryType":"issueCount"}`)
	if r.Error == nil || len(r.Frames) != 0 {
		t.Fatalf("got %v, %d frames", r.Error, len(r.Frames))
	}
}

func TestEventCountFailureIsNotCached(t *testing.T) {
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	f.status, f.body = 500, `{"detail":"boom"}`
	now := countsNow
	ds := f.datasource(t, &now)
	if r := runCount(t, ds, `{"queryType":"eventCount","project":1}`); r.Error == nil {
		t.Fatal("no error")
	}
	f.mu.Lock()
	f.status, f.body = 0, ""
	f.mu.Unlock()
	r := runCount(t, ds, `{"queryType":"eventCount","project":1}`)
	if got := valueOf(t, r, "Events", data.Labels{"project_id": "1"}); got != 1 {
		t.Fatalf("count %d, want 1", got)
	}
	if n := len(f.requests()); n != 2 {
		t.Fatalf("%d requests, want 2", n)
	}
}

func TestEventCountMakesOneCall(t *testing.T) {
	f := newFakeHourCounts(t, "2026-11-15T13:00:00Z")
	now := countsNow
	runCount(t, f.datasource(t, &now), `{"queryType":"eventCount","issue":"X-1","hours":24}`)
	if n := len(f.requests()); n != 1 {
		t.Fatalf("%d requests, want 1", n)
	}
}
