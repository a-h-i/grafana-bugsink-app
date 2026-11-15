// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func TestNewIssueStep(t *testing.T) {
	from := utc("2026-11-15T10:00:00Z")
	tests := []struct {
		name     string
		rng      time.Duration
		interval time.Duration
		want     time.Duration
	}{
		{"one hour, one second", time.Hour, time.Second, time.Minute},
		{"a week, ten minutes", 7 * 24 * time.Hour, 10 * time.Minute, 10 * time.Minute},
		{"a week, thirty seconds", 7 * 24 * time.Hour, 30 * time.Second, 7 * time.Minute},
		{"ninety days, two hours", 90 * 24 * time.Hour, 2 * time.Hour, 2 * time.Hour},
		{"one hour, ninety seconds", time.Hour, 90 * time.Second, 2 * time.Minute},
		{"zero interval", time.Hour, 0, time.Minute},
		{"negative interval", time.Hour, -time.Hour, time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := newIssueStep(from, from.Add(tc.rng), tc.interval); got != tc.want {
				t.Fatalf("step %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNewIssuePointsAreBounded checks the points of the window for ranges and intervals that could blow it up.
func TestNewIssuePointsAreBounded(t *testing.T) {
	const maxInterval = time.Duration(1<<63 - 1)
	froms := []time.Time{utc("2026-11-15T10:00:00Z"), utc("2026-11-15T10:00:59Z"), utc("2026-11-15T10:07:31Z"), {}}
	ranges := []time.Duration{-time.Hour, 0, time.Second, time.Hour, 1440 * time.Minute, 1440*time.Minute + time.Second,
		7 * 24 * time.Hour, 3 * 365 * 24 * time.Hour, 200 * 365 * 24 * time.Hour}
	intervals := []time.Duration{-1, 0, time.Millisecond, time.Second, 61 * time.Second, time.Hour, 1000 * time.Hour, maxInterval}
	for _, from := range froms {
		for _, rng := range ranges {
			for _, interval := range intervals {
				to := from.Add(rng)
				start, step := newIssueWindow(from, to, interval)
				if !to.After(from) {
					continue
				}
				if step < time.Minute || step%time.Minute != 0 {
					t.Fatalf("from %v range %v interval %v: step %v", from, rng, interval, step)
				}
				starts, totals := newIssueSeries(nil, start, to, step)
				if len(starts) > maxNewIssuePoints || len(starts) != len(totals) {
					t.Fatalf("from %v range %v interval %v: %d points", from, rng, interval, len(starts))
				}
			}
		}
	}
}

func TestNewIssueSeries(t *testing.T) {
	day := utc("2026-11-15T00:00:00Z")
	at := func(hm string) time.Time { return utc("2026-11-15T" + hm + "Z") }
	issue := func(first time.Time) bugsink.Issue { return bugsink.Issue{FirstSeen: first} }
	step := 10 * time.Minute
	tests := []struct {
		name   string
		issues []bugsink.Issue
		to     time.Time
		starts []time.Time
		totals []int64
	}{
		{
			name: "boundaries, edges and issues outside the window",
			issues: []bugsink.Issue{
				issue(at("00:01:00")), issue(at("00:09:00")), issue(at("00:15:00")), issue(at("00:24:59")),
				issue(at("00:25:00")), issue(day.Add(-time.Minute)),
			},
			to:     at("00:25:00"),
			starts: []time.Time{at("00:00:00"), at("00:10:00"), at("00:20:00")},
			totals: []int64{2, 1, 1},
		},
		{
			name:   "an issue on a boundary belongs to the step that starts there",
			issues: []bugsink.Issue{issue(at("00:00:00")), issue(at("00:10:00")), issue(at("00:10:00"))},
			to:     at("00:20:00"),
			starts: []time.Time{at("00:00:00"), at("00:10:00")},
			totals: []int64{1, 2},
		},
		{
			name:   "no issues is zeros",
			to:     at("00:25:00"),
			starts: []time.Time{at("00:00:00"), at("00:10:00"), at("00:20:00")},
			totals: []int64{0, 0, 0},
		},
		{
			name:   "a range ending on a boundary has no step starting there",
			to:     at("00:20:00"),
			starts: []time.Time{at("00:00:00"), at("00:10:00")},
			totals: []int64{0, 0},
		},
		{
			name:   "future and zero first seen are not counted",
			issues: []bugsink.Issue{issue(at("05:00:00")), issue(time.Time{}), issue(at("00:05:00"))},
			to:     at("00:20:00"),
			starts: []time.Time{at("00:00:00"), at("00:10:00")},
			totals: []int64{1, 0},
		},
		{
			name:   "a first seen in another time zone lands in its UTC step",
			issues: []bugsink.Issue{issue(time.Date(2026, 11, 15, 2, 12, 0, 0, time.FixedZone("x", 2*3600)))},
			to:     at("00:20:00"),
			starts: []time.Time{at("00:00:00"), at("00:10:00")},
			totals: []int64{0, 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starts, totals := newIssueSeries(tc.issues, day, tc.to, step)
			if !reflect.DeepEqual(starts, tc.starts) || !reflect.DeepEqual(totals, tc.totals) {
				t.Fatalf("got %v %v, want %v %v", starts, totals, tc.starts, tc.totals)
			}
		})
	}
	t.Run("the sum is the number of issues first seen in the window", func(t *testing.T) {
		var issues []bugsink.Issue
		inside := 0
		start, to := at("00:00:00"), at("03:00:00")
		for s := -3600; s < 4*3600; s += 37 {
			first := start.Add(time.Duration(s) * time.Second)
			issues = append(issues, issue(first))
			if !first.Before(start) && first.Before(to) {
				inside++
			}
		}
		_, totals := newIssueSeries(issues, start, to, step)
		var sum int64
		for _, n := range totals {
			sum += n
		}
		if sum != int64(inside) {
			t.Fatalf("sum %d, want %d", sum, inside)
		}
	})
}

var newIssuesNow = utc("2026-11-15T10:36:00Z")

// issueFirstLast is an issue answer with the first_seen value as raw JSON.
func issueFirstLast(friendly, firstJSON string, last time.Time) string {
	return strings.Replace(issueAt(friendly, last), `"first_seen":"2026-01-01T00:00:00Z"`, `"first_seen":`+firstJSON, 1)
}

func firstSeenAt(friendly string, first, last time.Time) string {
	return issueFirstLast(friendly, fmt.Sprintf("%q", first.Format(time.RFC3339Nano)), last)
}

func newIssuesDatasource(t *testing.T, f *fakeBugsink) *Datasource {
	ds := f.datasource(t)
	ds.now = func() time.Time { return newIssuesNow }
	return ds
}

func runNewIssues(t *testing.T, ds *Datasource, from, to time.Time, interval time.Duration, model string) backend.DataResponse {
	t.Helper()
	req := &backend.QueryDataRequest{
		Headers: map[string]string{"FromAlert": "true"},
		Queries: []backend.DataQuery{{
			RefID: "A", JSON: json.RawMessage(model), Interval: interval, TimeRange: backend.TimeRange{From: from, To: to},
		}},
	}
	resp, err := ds.QueryData(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return resp.Responses["A"]
}

func seriesOf(t *testing.T, r backend.DataResponse) (starts []time.Time, totals []int64) {
	t.Helper()
	frame := frameOf(t, r)
	for i := 0; i < frame.Fields[0].Len(); i++ {
		starts = append(starts, frame.Fields[0].At(i).(time.Time))
		totals = append(totals, frame.Fields[1].At(i).(int64))
	}
	return starts, totals
}

func TestNewIssuesQuery(t *testing.T) {
	from, to := utc("2026-11-15T10:05:00Z"), utc("2026-11-15T10:35:00Z")
	at := func(hm string) time.Time { return utc("2026-11-15T" + hm + "Z") }
	f := newFakeBugsink(t)
	f.pages = []string{
		strings.Join([]string{
			firstSeenAt("X-1", at("10:31:00"), at("10:33:00")),
			firstSeenAt("X-2", at("10:02:00"), at("10:20:00")),
			firstSeenAt("X-3", at("10:12:00"), at("10:15:00")),
			firstSeenAt("X-4", at("08:00:00"), at("10:10:00")),
			firstSeenAt("X-5", at("09:00:00"), at("09:59:00")),
		}, ","),
		firstSeenAt("X-6", at("10:03:00"), at("10:04:00")),
	}
	ds := newIssuesDatasource(t, f)
	r := runNewIssues(t, ds, from, to, 10*time.Minute, `{"queryType":"newIssues","project":1,"q":"x","state":"resolved"}`)
	starts, totals := seriesOf(t, r)
	wantStarts := []time.Time{at("10:00:00"), at("10:10:00"), at("10:20:00"), at("10:30:00")}
	if !reflect.DeepEqual(starts, wantStarts) || !reflect.DeepEqual(totals, []int64{1, 1, 0, 1}) {
		t.Fatalf("got %v %v", starts, totals)
	}
	reqs := f.issueRequests()
	if len(reqs) != 1 {
		t.Fatalf("%d requests, want 1 (paging stops at the first issue last seen before the window)", len(reqs))
	}
	for k, want := range map[string]string{"project": "1", "state": "all", "q": "x", "sort": "last_seen", "order": "desc"} {
		if got := reqs[0].Get(k); got != want {
			t.Fatalf("%s=%q, want %q (%v)", k, got, want, url.Values(reqs[0]))
		}
	}
	frame := r.Frames[0]
	if frame.Name != "New issues" || frame.RefID != "A" || frame.Meta.Type != data.FrameTypeTimeSeriesMulti {
		t.Fatalf("frame %q ref %q meta %+v", frame.Name, frame.RefID, frame.Meta)
	}
	if !reflect.DeepEqual(frame.Fields[1].Labels, data.Labels{"project_id": "1"}) {
		t.Fatalf("labels %v", frame.Fields[1].Labels)
	}
	if frame.Fields[0].Config == nil || frame.Fields[0].Config.Interval != 600000 {
		t.Fatalf("config %+v", frame.Fields[0].Config)
	}
	if frame.Fields[1].Type() != data.FieldTypeInt64 || len(frame.Meta.Notices) != 0 {
		t.Fatalf("type %v notices %v", frame.Fields[1].Type(), frame.Meta.Notices)
	}
}

func TestNewIssuesAllProjectsHasNoLabel(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{""}
	ds := newIssuesDatasource(t, f)
	r := runNewIssues(t, ds, utc("2026-11-15T10:00:00Z"), utc("2026-11-15T10:20:00Z"), time.Minute, `{"queryType":"newIssues"}`)
	if labels := frameOf(t, r).Fields[1].Labels; len(labels) != 0 {
		t.Fatalf("labels %v", labels)
	}
}

func TestNewIssuesAllZeroIsNotEmpty(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{""}
	ds := newIssuesDatasource(t, f)
	r := runNewIssues(t, ds, utc("2026-11-15T09:00:00Z"), utc("2026-11-15T10:00:00Z"), time.Minute, `{"queryType":"newIssues","project":1}`)
	starts, totals := seriesOf(t, r)
	if len(starts) != 60 {
		t.Fatalf("%d points, want 60", len(starts))
	}
	for i, n := range totals {
		if n != 0 {
			t.Fatalf("point %d is %d", i, n)
		}
	}
}

func TestNewIssuesFirstSeenOutsideTheWindowIsNotCounted(t *testing.T) {
	at := func(hm string) time.Time { return utc("2026-11-15T" + hm + "Z") }
	f := newFakeBugsink(t)
	f.pages = []string{strings.Join([]string{
		firstSeenAt("X-1", at("09:30:00"), at("10:12:00")), // seen again inside the window
		firstSeenAt("X-2", at("10:31:00"), at("10:32:00")), // first seen at the last step
		firstSeenAt("X-3", at("11:30:00"), at("11:30:00")), // in the future
	}, ",")}
	ds := newIssuesDatasource(t, f)
	r := runNewIssues(t, ds, at("10:00:00"), at("10:35:00"), 10*time.Minute, `{"queryType":"newIssues"}`)
	_, totals := seriesOf(t, r)
	if !reflect.DeepEqual(totals, []int64{0, 0, 0, 1}) {
		t.Fatalf("totals %v", totals)
	}
}

func TestNewIssuesBadFirstSeenIsAnError(t *testing.T) {
	last := utc("2026-11-15T10:12:00Z")
	for name, issue := range map[string]string{
		"missing":          strings.Replace(issueAt("X-1", last), `"first_seen":"2026-01-01T00:00:00Z",`, "", 1),
		"null":             issueFirstLast("X-1", "null", last),
		"malformed":        issueFirstLast("X-1", `"yesterday"`, last),
		"no time zone":     issueFirstLast("X-1", `"2026-11-15T10:11:00"`, last),
		"not a string":     issueFirstLast("X-1", `12`, last),
		"empty":            issueFirstLast("X-1", `""`, last),
		"year zero":        issueFirstLast("X-1", `"0001-01-01T00:00:00Z"`, last),
		"one bad of three": issueAt("X-0", last) + "," + issueFirstLast("X-1", `"x"`, last),
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.pages = []string{issue}
			ds := newIssuesDatasource(t, f)
			r := runNewIssues(t, ds, utc("2026-11-15T10:00:00Z"), utc("2026-11-15T10:35:00Z"), 10*time.Minute, `{"queryType":"newIssues"}`)
			if r.Error == nil || len(r.Frames) != 0 || r.Status < 400 {
				t.Fatalf("got %v status %d, %d frames", r.Error, r.Status, len(r.Frames))
			}
		})
	}
}

func TestNewIssuesCap(t *testing.T) {
	const message = "More than 5000 issues were seen in the time range, so the new ones cannot be counted. Narrow the query with a project or a search, or shorten the time range."
	f := newFakeBugsink(t)
	f.endless = true
	ds := newIssuesDatasource(t, f)
	for round := 1; round <= 2; round++ {
		r := runNewIssues(t, ds, rangeFrom, rangeTo, time.Hour, `{"queryType":"newIssues"}`)
		if r.Error == nil || r.Error.Error() != message || r.Status != backend.StatusBadRequest || len(r.Frames) != 0 {
			t.Fatalf("round %d: %v status %d frames %d", round, r.Error, r.Status, len(r.Frames))
		}
		if n := len(f.issueRequests()); n != 20*round {
			t.Fatalf("round %d: %d requests, want %d", round, n, 20*round)
		}
	}
}

func TestNewIssuesAtTheCapAndOnePast(t *testing.T) {
	pages := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = firstSeenAt(fmt.Sprintf("X-%d", i+1), rangeTo.Add(-2*time.Hour), rangeTo.Add(-time.Hour))
		}
		return out
	}
	t.Run("a list ending on page 20 is a series", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.pages = pages(20)
		r := runNewIssues(t, newIssuesDatasource(t, f), rangeFrom, rangeTo, time.Hour, `{"queryType":"newIssues"}`)
		_, totals := seriesOf(t, r)
		var sum int64
		for _, n := range totals {
			sum += n
		}
		if sum != 20 || len(f.issueRequests()) != 20 {
			t.Fatalf("sum %d, %d requests", sum, len(f.issueRequests()))
		}
	})
	t.Run("a list with a 21st page is the cap error", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.pages = pages(21)
		r := runNewIssues(t, newIssuesDatasource(t, f), rangeFrom, rangeTo, time.Hour, `{"queryType":"newIssues"}`)
		if r.Error == nil || r.Status != backend.StatusBadRequest || len(r.Frames) != 0 {
			t.Fatalf("got %v status %d frames %d", r.Error, r.Status, len(r.Frames))
		}
		if n := len(f.issueRequests()); n != 20 {
			t.Fatalf("%d requests, want 20", n)
		}
	})
}

func TestNewIssuesBoundedWhateverTheRange(t *testing.T) {
	now := utc("2026-11-15T10:36:00Z")
	for name, c := range map[string]struct {
		from, to time.Time
		interval time.Duration
	}{
		"years":          {now.Add(-5 * 365 * 24 * time.Hour), now, time.Second},
		"huge interval":  {now.Add(-time.Hour), now, time.Duration(1<<63 - 1)},
		"zero interval":  {now.Add(-time.Hour), now, 0},
		"negative":       {now.Add(-time.Hour), now, -time.Minute},
		"misaligned":     {utc("2026-11-15T09:36:59Z"), now.Add(1440*time.Minute - time.Hour), time.Minute},
		"far in future":  {now, now.Add(100 * 365 * 24 * time.Hour), time.Minute},
		"time zero from": {time.Time{}, now, time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.pages = []string{""}
			r := runNewIssues(t, newIssuesDatasource(t, f), c.from, c.to, c.interval, `{"queryType":"newIssues"}`)
			starts, _ := seriesOf(t, r)
			if len(starts) == 0 || len(starts) > maxNewIssuePoints {
				t.Fatalf("%d points", len(starts))
			}
			if n := len(f.issueRequests()); n > 20 {
				t.Fatalf("%d requests", n)
			}
		})
	}
}

func TestNewIssuesEmptyRangeIsAnError(t *testing.T) {
	at := utc("2026-11-15T10:00:00Z")
	for name, c := range map[string]struct{ from, to time.Time }{
		"zero":     {at, at},
		"reversed": {at, at.Add(-time.Hour)},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := runNewIssues(t, newIssuesDatasource(t, f), c.from, c.to, time.Minute, `{"queryType":"newIssues"}`)
			if r.Error == nil || r.Error.Error() != "The time range must end after it starts." || r.Status != backend.StatusBadRequest || len(r.Frames) != 0 {
				t.Fatalf("got %v status %d frames %d", r.Error, r.Status, len(r.Frames))
			}
			if n := len(f.issueRequests()); n != 0 {
				t.Fatalf("%d requests", n)
			}
		})
	}
}

func TestNewIssuesUsesTheIssueCache(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{firstSeenAt("X-1", utc("2026-11-15T10:12:00Z"), utc("2026-11-15T10:13:00Z"))}
	ds := newIssuesDatasource(t, f)
	day := runQueries(t, ds, utc("2026-11-15T09:00:00Z"), utc("2026-11-15T10:35:00Z"), `{"state":"all","q":"x","project":1}`)
	if day.Responses["A"].Error != nil {
		t.Fatal(day.Responses["A"].Error)
	}
	r := runNewIssues(t, ds, utc("2026-11-15T10:05:00Z"), utc("2026-11-15T10:35:00Z"), 10*time.Minute, `{"queryType":"newIssues","project":1,"q":"x"}`)
	_, totals := seriesOf(t, r)
	if !reflect.DeepEqual(totals, []int64{0, 1, 0, 0}) {
		t.Fatalf("totals %v", totals)
	}
	if n := len(f.issueRequests()); n != 1 {
		t.Fatalf("%d list reads, want 1", n)
	}
}

func TestNewIssuesStartDoesNotFollowTheSecond(t *testing.T) {
	f := newFakeBugsink(t)
	f.pages = []string{firstSeenAt("X-1", utc("2026-11-15T10:12:00Z"), utc("2026-11-15T10:13:00Z"))}
	ds := f.datasource(t)
	model := `{"queryType":"newIssues","project":1}`
	var first []int64
	for i, offset := range []time.Duration{0, 4 * time.Second, 9 * time.Second} {
		now := utc("2026-11-15T10:36:07Z").Add(offset)
		ds.now = func() time.Time { return now }
		r := runNewIssues(t, ds, now.Add(-time.Hour), now, time.Minute, model)
		_, totals := seriesOf(t, r)
		if i == 0 {
			first = totals
		}
		if n := len(f.issueRequests()); n != 1 {
			t.Fatalf("evaluation %d: %d list reads, want 1", i, n)
		}
	}
	if len(first) != 61 {
		t.Fatalf("%d points", len(first))
	}
}

func TestNewIssuesFailure(t *testing.T) {
	t.Run("a closed server", func(t *testing.T) {
		f := newFakeBugsink(t)
		ds := newIssuesDatasource(t, f)
		f.srv.Close()
		r := runNewIssues(t, ds, utc("2026-11-15T10:05:00Z"), utc("2026-11-15T10:35:00Z"), 10*time.Minute, `{"queryType":"newIssues"}`)
		if r.Error == nil || !strings.HasPrefix(r.Error.Error(), "Cannot reach Bugsink: ") || r.Status != backend.StatusBadGateway || len(r.Frames) != 0 {
			t.Fatalf("got %v status %d frames %d", r.Error, r.Status, len(r.Frames))
		}
	})
	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"refusing":      {403, `{"detail":"This token is not allowed."}`},
		"server error":  {500, `{"detail":"boom"}`},
		"not bugsink":   {200, `<html>hi</html>`},
		"empty answer":  {200, ` `},
		"wrong results": {200, `{"next":null,"results":"x"}`},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.issuesStatus, f.issuesBody = c.status, c.body
			if c.status == 200 {
				f.issuesStatus = 0
				f.pages = []string{`"not an issue"`}
			}
			ds := newIssuesDatasource(t, f)
			r := runNewIssues(t, ds, utc("2026-11-15T10:05:00Z"), utc("2026-11-15T10:35:00Z"), 10*time.Minute, `{"queryType":"newIssues"}`)
			if r.Error == nil || len(r.Frames) != 0 || r.Status < 400 {
				t.Fatalf("got %v status %d frames %d", r.Error, r.Status, len(r.Frames))
			}
		})
	}
}

func TestNewIssuesFailureIsNotCached(t *testing.T) {
	f := newFakeBugsink(t)
	f.issuesStatus, f.issuesBody = 500, `{"detail":"boom"}`
	ds := newIssuesDatasource(t, f)
	from, to := utc("2026-11-15T10:05:00Z"), utc("2026-11-15T10:35:00Z")
	if r := runNewIssues(t, ds, from, to, 10*time.Minute, `{"queryType":"newIssues"}`); r.Error == nil {
		t.Fatal("no error")
	}
	f.mu.Lock()
	f.issuesStatus = 0
	f.pages = []string{firstSeenAt("X-1", utc("2026-11-15T10:12:00Z"), utc("2026-11-15T10:13:00Z"))}
	f.mu.Unlock()
	r := runNewIssues(t, ds, from, to, 10*time.Minute, `{"queryType":"newIssues"}`)
	_, totals := seriesOf(t, r)
	if !reflect.DeepEqual(totals, []int64{0, 1, 0, 0}) {
		t.Fatalf("totals %v", totals)
	}
	if n := len(f.issueRequests()); n != 2 {
		t.Fatalf("%d requests, want 2", n)
	}
}
