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
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// fakeCounts serves projects/ and the event-counts reads; every hourly bucket counts its hour of the day plus one.
type fakeCounts struct {
	mu             sync.Mutex
	paths          []string
	counts         []url.Values
	status         int
	body           string
	projectsStatus int
	echoAuth       bool
	delay          time.Duration
	gate           chan struct{} // when set, event-counts requests wait for it to close
	held, maxHeld  atomic.Int64
	srv            *httptest.Server
}

func newFakeCounts(t *testing.T) *fakeCounts {
	t.Helper()
	f := &fakeCounts{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCounts) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	status, body, projectsStatus, delay := f.status, f.body, f.projectsStatus, f.delay
	if f.echoAuth {
		body = "<html>" + r.Header.Get("Authorization") + "</html>"
	}
	isCounts := strings.HasSuffix(r.URL.Path, "/event-counts/")
	if isCounts {
		f.counts = append(f.counts, r.URL.Query())
	}
	gate := f.gate
	f.mu.Unlock()
	if gate != nil && isCounts {
		n := f.held.Add(1)
		for m := f.maxHeld.Load(); n > m && !f.maxHeld.CompareAndSwap(m, n); m = f.maxHeld.Load() {
		}
		<-gate
		f.held.Add(-1)
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	switch {
	case r.URL.Path == "/api/canonical/0/projects/":
		if projectsStatus != 0 {
			w.WriteHeader(projectsStatus)
			_, _ = fmt.Fprint(w, `{"detail":"nope"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[{"id":1,"name":"sample-python"}]}`)
	case isCounts:
		if status != 0 {
			w.WriteHeader(status)
			_, _ = fmt.Fprint(w, body)
			return
		}
		_, _ = fmt.Fprint(w, countsAnswer(r.URL.Query()))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"detail":"Not found."}`)
	}
}

func countsAnswer(q url.Values) string {
	start, _ := time.Parse(time.RFC3339, q.Get("start"))
	end := time.Date(2026, 11, 15, 14, 0, 0, 0, time.UTC)
	if e := q.Get("end"); e != "" {
		end, _ = time.Parse(time.RFC3339, e)
	}
	if q.Get("start") == "" {
		start = end.Add(-24 * time.Hour)
	}
	var buckets []string
	for h := start; h.Before(end); h = h.Add(time.Hour) {
		buckets = append(buckets, fmt.Sprintf(`{"timestamp":%q,"count":%d}`, h.Format(time.RFC3339), h.Hour()+1))
	}
	return fmt.Sprintf(`{"start":%q,"end":%q,"interval_seconds":3600,"buckets":[%s]}`,
		start.Format(time.RFC3339), end.Format(time.RFC3339), strings.Join(buckets, ","))
}

func (f *fakeCounts) countRequests() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.counts...)
}

func (f *fakeCounts) pathCount(path string) int {
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

func (f *fakeCounts) datasource(t *testing.T) *Datasource {
	ds := newInstance(t, f.srv.URL, secretToken, `{"browserUrl":"http://b"}`)
	ds.now = func() time.Time { return volumeNow }
	return ds
}

func runVolume(t *testing.T, ds *Datasource, model string, from, to time.Time, interval time.Duration) backend.DataResponse {
	t.Helper()
	req := &backend.QueryDataRequest{Queries: []backend.DataQuery{{
		RefID: "A", JSON: json.RawMessage(model), Interval: interval,
		TimeRange: backend.TimeRange{From: from, To: to},
	}}}
	resp, err := ds.QueryData(context.Background(), req)
	if err != nil {
		t.Fatalf("QueryData returned error: %v", err)
	}
	return resp.Responses["A"]
}

func sevenDays(t *testing.T, ds *Datasource, model string, interval time.Duration) backend.DataResponse {
	t.Helper()
	return runVolume(t, ds, model, volumeNow.Add(-7*24*time.Hour), volumeNow, interval)
}

func noticeTexts(f *data.Frame) []string {
	if f.Meta == nil {
		return nil
	}
	var out []string
	for _, n := range f.Meta.Notices {
		out = append(out, fmt.Sprintf("%d:%s", n.Severity, n.Text))
	}
	return out
}

func totalsOf(f *data.Frame) []int64 {
	field := f.Fields[1]
	out := make([]int64, field.Len())
	for i := range out {
		out[i] = field.At(i).(int64)
	}
	return out
}

func expectEmptyFrame(t *testing.T, r backend.DataResponse) *data.Frame {
	t.Helper()
	frame := frameOf(t, r)
	if len(frame.Fields) != 2 || frame.Fields[0].Name != "time" || frame.Fields[1].Name != "count" || frame.Fields[0].Len() != 0 {
		t.Fatalf("not an empty volume frame: %d fields, %d rows", len(frame.Fields), frame.Fields[0].Len())
	}
	return frame
}

const (
	stepNoticeText     = "0:Each point counts one hour of events: Bugsink counts events per hour."
	retentionNotice    = "1:Bugsink keeps hourly event counts for 90 days, so the series starts later than the time range."
	namesNothingNotice = "0:An event volume query needs a project or an issue."
)

func TestVolumeQueryProject(t *testing.T) {
	f := newFakeCounts(t)
	frame := frameOf(t, sevenDays(t, f.datasource(t), `{"queryType":"volume","project":1}`, 10*time.Minute))

	reqs := f.countRequests()
	if len(reqs) != 1 || reqs[0].Get("start") != "2026-11-08T13:00:00Z" || reqs[0].Has("end") {
		t.Fatalf("requests %v", reqs)
	}
	if f.pathCount("/api/canonical/0/projects/1/event-counts/") != 1 {
		t.Fatalf("paths %v", f.paths)
	}
	if frame.Name != "sample-python" || frame.RefID != "A" || frame.Meta.Type != data.FrameTypeTimeSeriesMulti {
		t.Fatalf("frame %q %q %v", frame.Name, frame.RefID, frame.Meta)
	}
	count := frame.Fields[1]
	if count.Config.DisplayNameFromDS != "sample-python" || count.Labels["project"] != "sample-python" || len(count.Labels) != 1 {
		t.Fatalf("field %+v labels %v", count.Config, count.Labels)
	}
	totals := totalsOf(frame)
	if len(totals) != 169 || totals[0] != 14 || totals[168] != 14 {
		t.Fatalf("%d totals, first %d last %d", len(totals), totals[0], totals[168])
	}
	if first := frame.Fields[0].At(0).(time.Time); !first.Equal(time.Date(2026, 11, 8, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("first point %v", first)
	}
	if frame.Fields[0].Config.Interval != 3600000 {
		t.Fatalf("interval %v", frame.Fields[0].Config.Interval)
	}
	if got := noticeTexts(frame); len(got) != 1 || got[0] != stepNoticeText {
		t.Fatalf("notices %v", got)
	}
}

func TestVolumeQueryIssue(t *testing.T) {
	f := newFakeCounts(t)
	frame := frameOf(t, sevenDays(t, f.datasource(t), `{"queryType":"volume","issue":"SAMPLE-PYTHON-2","project":1}`, time.Hour))

	if f.pathCount("/api/canonical/0/issues/SAMPLE-PYTHON-2/event-counts/") != 1 || len(f.paths) != 1 {
		t.Fatalf("paths %v", f.paths)
	}
	count := frame.Fields[1]
	if frame.Name != "SAMPLE-PYTHON-2" || count.Config.DisplayNameFromDS != "SAMPLE-PYTHON-2" ||
		count.Labels["issue"] != "SAMPLE-PYTHON-2" || len(count.Labels) != 1 {
		t.Fatalf("frame %q field %+v labels %v", frame.Name, count.Config, count.Labels)
	}
	if got := noticeTexts(frame); len(got) != 0 {
		t.Fatalf("notices %v", got)
	}
}

func TestVolumeQueryPastRange(t *testing.T) {
	f := newFakeCounts(t)
	from, to := time.Date(2026, 11, 10, 10, 30, 0, 0, time.UTC), time.Date(2026, 11, 10, 12, 30, 0, 0, time.UTC)
	frame := frameOf(t, runVolume(t, f.datasource(t), `{"queryType":"volume","project":1}`, from, to, time.Hour))

	reqs := f.countRequests()
	if len(reqs) != 1 || reqs[0].Get("start") != "2026-11-10T10:00:00Z" || reqs[0].Get("end") != "2026-11-10T13:00:00Z" {
		t.Fatalf("requests %v", reqs)
	}
	if got := totalsOf(frame); len(got) != 3 {
		t.Fatalf("totals %v", got)
	}
}

func TestVolumeQuerySteps(t *testing.T) {
	f := newFakeCounts(t)
	frame := frameOf(t, runVolume(t, f.datasource(t), `{"queryType":"volume","project":1}`,
		volumeNow.Add(-24*time.Hour), volumeNow, 3*time.Hour))

	// hours 12..14 of the 14th, then every third hour to 12..13 of the 15th; a count is its hour plus one
	want := []int64{13 + 14 + 15, 16 + 17 + 18, 19 + 20 + 21, 22 + 23 + 24, 1 + 2 + 3, 4 + 5 + 6, 7 + 8 + 9, 10 + 11 + 12, 13 + 14}
	got := totalsOf(frame)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("totals %v, want %v", got, want)
	}
	if frame.Fields[0].Config.Interval != 10800000 {
		t.Fatalf("interval %v", frame.Fields[0].Config.Interval)
	}
	if got := noticeTexts(frame); len(got) != 0 {
		t.Fatalf("notices %v", got)
	}
	if reqs := f.countRequests(); len(reqs) != 1 || reqs[0].Get("start") != "2026-11-14T12:00:00Z" || reqs[0].Has("end") {
		t.Fatalf("requests %v", reqs)
	}
}

func TestVolumeQueryOutsideRetention(t *testing.T) {
	f := newFakeCounts(t)
	ds := f.datasource(t)

	frame := frameOf(t, runVolume(t, ds, `{"queryType":"volume","project":1}`, volumeNow.Add(-180*24*time.Hour), volumeNow, 3*time.Hour))
	reqs := f.countRequests()
	if len(reqs) != 1 || reqs[0].Get("start") != "2026-08-17T15:00:00Z" {
		t.Fatalf("requests %v", reqs)
	}
	if got := noticeTexts(frame); len(got) != 1 || got[0] != retentionNotice {
		t.Fatalf("notices %v", got)
	}
	if n := frame.Fields[0].Len(); n != 720 {
		t.Fatalf("%d points", n)
	}

	older := expectEmptyFrame(t, runVolume(t, ds, `{"queryType":"volume","project":1}`,
		volumeNow.Add(-200*24*time.Hour), volumeNow.Add(-100*24*time.Hour), time.Hour))
	if got := noticeTexts(older); len(got) != 1 || got[0] != retentionNotice {
		t.Fatalf("older notices %v", got)
	}
	future := expectEmptyFrame(t, runVolume(t, ds, `{"queryType":"volume","project":1}`,
		volumeNow.Add(3*time.Hour), volumeNow.Add(6*time.Hour), time.Hour))
	if got := noticeTexts(future); len(got) != 0 {
		t.Fatalf("future notices %v", got)
	}
	if len(f.countRequests()) != 1 {
		t.Fatalf("an empty window made a request: %v", f.countRequests())
	}
}

func TestVolumeQueryNamesNothing(t *testing.T) {
	f := newFakeCounts(t)
	frame := expectEmptyFrame(t, sevenDays(t, f.datasource(t), `{"queryType":"volume"}`, time.Hour))
	if got := noticeTexts(frame); len(got) != 1 || got[0] != namesNothingNotice {
		t.Fatalf("notices %v", got)
	}
	if len(f.paths) != 0 {
		t.Fatalf("requests %v", f.paths)
	}
}

func TestVolumeQueryValidation(t *testing.T) {
	f := newFakeCounts(t)
	ds := f.datasource(t)
	for _, issue := range []string{"a/b", "..", "%2F", "SAMPLE PYTHON"} {
		model, _ := json.Marshal(map[string]any{"queryType": "volume", "issue": issue})
		r := sevenDays(t, ds, string(model), time.Hour)
		if want := fmt.Sprintf("%q is not an issue id.", issue); r.Error == nil || r.Error.Error() != want || r.Status != backend.StatusBadRequest {
			t.Fatalf("%q: got %v status %d, want %q 400", issue, r.Error, r.Status, want)
		}
	}
	r := sevenDays(t, ds, `{"queryType":"volume","project":"x"}`, time.Hour)
	if r.Error == nil || r.Error.Error() != "The project must be a number." || r.Status != backend.StatusBadRequest {
		t.Fatalf("got %v status %d", r.Error, r.Status)
	}
	if len(f.paths) != 0 {
		t.Fatalf("requests %v", f.paths)
	}
}

func TestVolumeQueryErrors(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		status  int
		body    string
		message string
		code    backend.Status
	}{
		{"project forbidden", `{"queryType":"volume","project":2}`, 403, `{"detail":"This token is not allowed to access this project."}`,
			"Bugsink refused the request: This token is not allowed to access this project.", 403},
		{"capability", `{"queryType":"volume","issue":"A-1"}`, 403, `{"detail":"This token does not have the required capability: issues:read."}`,
			"The API token lacks issues:read.", 403},
		{"issue missing", `{"queryType":"volume","issue":"A-1"}`, 404, `{"detail":"No Issue matches the given query."}`,
			"Bugsink refused the request: No Issue matches the given query.", 404},
		{"end too late", `{"queryType":"volume","project":1}`, 400, `{"end":["Cannot be later than the end of the current hour."]}`,
			"Bugsink refused the request: end: Cannot be later than the end of the current hour.", 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeCounts(t)
			f.status, f.body = c.status, c.body
			r := sevenDays(t, f.datasource(t), c.model, time.Hour)
			if r.Error == nil || r.Error.Error() != c.message || r.Status != c.code || r.ErrorSource != backend.ErrorSourceDownstream {
				t.Fatalf("got %v status %d source %q", r.Error, r.Status, r.ErrorSource)
			}
		})
	}
}

func TestVolumeErrorsDoNotCarryTheToken(t *testing.T) {
	f := newFakeCounts(t)
	f.status, f.echoAuth = 502, true
	r := sevenDays(t, f.datasource(t), `{"queryType":"volume","issue":"A-1"}`, time.Hour)
	if r.Error == nil || strings.Contains(r.Error.Error(), secretToken) {
		t.Fatalf("error %v", r.Error)
	}
}

func TestVolumeCache(t *testing.T) {
	model := `{"queryType":"volume","project":1}`

	t.Run("same query twice", func(t *testing.T) {
		f := newFakeCounts(t)
		ds := f.datasource(t)
		sevenDays(t, ds, model, time.Hour)
		sevenDays(t, ds, model, time.Hour)
		if n := len(f.countRequests()); n != 1 {
			t.Fatalf("%d requests", n)
		}
	})

	t.Run("steps share the window", func(t *testing.T) {
		f := newFakeCounts(t)
		ds := f.datasource(t)
		from, to := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
		for _, interval := range []time.Duration{10 * time.Minute, time.Hour} {
			if r := runVolume(t, ds, model, from, to, interval); r.Error != nil {
				t.Fatal(r.Error)
			}
		}
		if n := len(f.countRequests()); n != 1 {
			t.Fatalf("%d requests", n)
		}
	})

	t.Run("windows, projects and issues do not share", func(t *testing.T) {
		f := newFakeCounts(t)
		ds := f.datasource(t)
		sevenDays(t, ds, model, time.Hour)
		runVolume(t, ds, model, volumeNow.Add(-6*24*time.Hour), volumeNow, time.Hour)
		sevenDays(t, ds, `{"queryType":"volume","project":2}`, time.Hour)
		sevenDays(t, ds, `{"queryType":"volume","issue":"A-1"}`, time.Hour)
		sevenDays(t, ds, `{"queryType":"volume","issue":"A-2"}`, time.Hour)
		if n := len(f.countRequests()); n != 5 {
			t.Fatalf("%d requests, want 5", n)
		}
	})

	t.Run("two data sources do not share", func(t *testing.T) {
		f := newFakeCounts(t)
		sevenDays(t, f.datasource(t), model, time.Hour)
		sevenDays(t, f.datasource(t), model, time.Hour)
		if n := len(f.countRequests()); n != 2 {
			t.Fatalf("%d requests, want 2", n)
		}
	})

	t.Run("expires after a minute", func(t *testing.T) {
		f := newFakeCounts(t)
		ds := f.datasource(t)
		clock := &fakeClock{t: time.Unix(1000, 0)}
		ds.countCache.now = clock.now
		sevenDays(t, ds, model, time.Hour)
		clock.advance(59 * time.Second)
		sevenDays(t, ds, model, time.Hour)
		if n := len(f.countRequests()); n != 1 {
			t.Fatalf("%d requests after 59s", n)
		}
		clock.advance(2 * time.Second)
		sevenDays(t, ds, model, time.Hour)
		if n := len(f.countRequests()); n != 2 {
			t.Fatalf("%d requests after 61s", n)
		}
	})

	t.Run("an error is not stored", func(t *testing.T) {
		f := newFakeCounts(t)
		ds := f.datasource(t)
		f.status, f.body = 502, `{"detail":"bad gateway"}`
		if r := sevenDays(t, ds, model, time.Hour); r.Error == nil {
			t.Fatal("no error")
		}
		f.mu.Lock()
		f.status = 0
		f.mu.Unlock()
		if r := sevenDays(t, ds, model, time.Hour); r.Error != nil {
			t.Fatal(r.Error)
		}
		if n := len(f.countRequests()); n != 2 {
			t.Fatalf("%d requests, want 2", n)
		}
	})

	t.Run("holds at most 128 windows", func(t *testing.T) {
		f := newFakeCounts(t)
		ds := f.datasource(t)
		for i := 0; i < countCacheMaxEntries+10; i++ {
			sevenDays(t, ds, fmt.Sprintf(`{"queryType":"volume","issue":"A-%d"}`, i), time.Hour)
		}
		before := len(f.countRequests())
		sevenDays(t, ds, `{"queryType":"volume","issue":"A-0"}`, time.Hour)
		if n := len(f.countRequests()) - before; n != 1 {
			t.Fatalf("the first window was kept: %d more requests, want 1", n)
		}
	})
}

// The test's value is the -race run: parallel cold misses share the cache.
func TestVolumeCacheParallelQueries(t *testing.T) {
	f := newFakeCounts(t)
	f.delay = 20 * time.Millisecond
	ds := f.datasource(t)
	const parallel = 8
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := sevenDays(t, ds, `{"queryType":"volume","project":1}`, time.Hour); r.Error != nil {
				t.Error(r.Error)
			}
		}()
	}
	wg.Wait()
	if n := len(f.countRequests()); n < 1 || n > parallel {
		t.Fatalf("%d requests for %d parallel queries", n, parallel)
	}
	before := len(f.countRequests())
	sevenDays(t, ds, `{"queryType":"volume","project":1}`, time.Hour)
	if n := len(f.countRequests()); n != before {
		t.Fatalf("a cached window made %d more requests", n-before)
	}
}

func TestVolumeProjectNameUnreadable(t *testing.T) {
	f := newFakeCounts(t)
	f.projectsStatus = 403
	frame := frameOf(t, sevenDays(t, f.datasource(t), `{"queryType":"volume","project":1}`, time.Hour))
	if frame.Name != "1" || frame.Fields[1].Labels["project"] != "1" || frame.Fields[1].Config.DisplayNameFromDS != "1" {
		t.Fatalf("name %q labels %v", frame.Name, frame.Fields[1].Labels)
	}
}

func TestIdenticalColdQueriesShareOneCall(t *testing.T) {
	const parallel = 8
	runAll := func(ds *Datasource, run func() backend.DataResponse, release func(), shared func() int) []backend.DataResponse {
		results := make([]backend.DataResponse, parallel)
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func() { defer wg.Done(); results[i] = run() }()
		}
		eventually(t, "7 callers sharing one call", func() bool { return shared() == parallel-1 })
		release()
		wg.Wait()
		return results
	}
	assertEqualFrames := func(t *testing.T, results []backend.DataResponse) {
		t.Helper()
		want := frameOf(t, results[0])
		for _, r := range results[1:] {
			if !reflect.DeepEqual(want, frameOf(t, r)) {
				t.Fatal("frames differ")
			}
		}
	}

	t.Run("volume", func(t *testing.T) {
		f := newFakeCounts(t)
		f.gate = make(chan struct{})
		ds := f.datasource(t)
		results := runAll(ds, func() backend.DataResponse {
			return sevenDays(t, ds, `{"queryType":"volume","project":1}`, time.Hour)
		}, func() { close(f.gate) }, ds.countFlight.totalDups)
		assertEqualFrames(t, results)
		if n := len(f.countRequests()); n != 1 {
			t.Fatalf("%d event-counts requests, want 1", n)
		}
	})
	t.Run("issues", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.pages = []string{issueAt("A-1", rangeTo.Add(-time.Hour))}
		f.issuesGate = make(chan struct{})
		ds := f.datasource(t)
		results := runAll(ds, func() backend.DataResponse {
			return runQuery(t, ds, `{"queryType":"issues"}`)
		}, func() { close(f.issuesGate) }, ds.issueFlight.totalDups)
		assertEqualFrames(t, results)
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.requests) != 1 {
			t.Fatalf("%d issue list requests, want 1", len(f.requests))
		}
	})
}

func TestInstanceLimitsConcurrentCalls(t *testing.T) {
	const queries = 20
	f := newFakeCounts(t)
	f.gate = make(chan struct{})
	ds := f.datasource(t)
	results := make([]backend.DataResponse, queries)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = sevenDays(t, ds, fmt.Sprintf(`{"queryType":"volume","issue":"A-%d"}`, i), time.Hour)
		}()
	}
	eventually(t, "8 requests held", func() bool { return f.held.Load() == maxConcurrentCalls })
	time.Sleep(50 * time.Millisecond)
	if m := f.maxHeld.Load(); m != maxConcurrentCalls {
		t.Fatalf("Bugsink held %d requests at once, want %d", m, maxConcurrentCalls)
	}
	close(f.gate)
	wg.Wait()
	for i, r := range results {
		if r.Error != nil {
			t.Fatalf("query %d: %v", i, r.Error)
		}
	}
}

func TestRefusedCountsAreLogged(t *testing.T) {
	logs := captureLogs(t)
	f := newFakeCounts(t)
	f.status, f.body = 200, `{"start":"2026-11-14T14:00:00Z","end":"2026-11-15T14:00:00Z","interval_seconds":86400,"buckets":[]}`
	r := sevenDays(t, f.datasource(t), `{"queryType":"volume","project":1}`, time.Hour)
	if r.Error == nil || r.Error.Error() != "The answer did not look like a Bugsink API." || r.Status != backend.StatusBadGateway {
		t.Fatalf("panel message %v status %d", r.Error, r.Status)
	}
	var reasons []string
	for _, l := range logs.all() {
		if l.msg == "Event counts refused" {
			if l.level != "debug" || len(l.args) != 2 || l.args[0] != "reason" {
				t.Fatalf("log line %s", l)
			}
			reasons = append(reasons, fmt.Sprint(l.args[1]))
		}
	}
	if len(reasons) != 1 || !strings.Contains(reasons[0], "are not hourly") {
		t.Fatalf("refused-count lines %v, want one holding %q", reasons, "are not hourly")
	}
	logs.assertNoToken(t)
}
