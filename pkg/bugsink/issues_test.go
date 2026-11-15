// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func issueJSON(friendly string, lastSeen string) string {
	return fmt.Sprintf(`{"id":"0b1e1c9e-1a2b-4c3d-8e4f-0123456789ab","friendly_id":%q,"project":1,`+
		`"digest_order":1,"first_seen":"2026-01-01T00:00:00Z","last_seen":%q,"digested_event_count":3,`+
		`"stored_event_count":2,"calculated_type":"ValueError","calculated_value":"boom","transaction":"/x",`+
		`"is_resolved":false,"is_resolved_by_next_release":false,"is_muted":false,"unmute_after":null}`,
		friendly, lastSeen)
}

func page(next string, issues ...string) string {
	n := "null"
	if next != "" {
		n = fmt.Sprintf("%q", next)
	}
	return fmt.Sprintf(`{"next":%s,"previous":null,"results":[%s]}`, n, strings.Join(issues, ","))
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "tok", nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListIssuesSendsFilter(t *testing.T) {
	var got url.Values
	var path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got, path = r.URL.Query(), r.URL.Path
		_, _ = fmt.Fprint(w, page(""))
	})
	q := `environment:prod "a b" &x=1 #y`
	_, err := c.ListIssues(context.Background(),
		IssueFilter{ProjectID: 1, State: StateUnresolved, Query: q, Sort: SortLastSeen, Order: Desc}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{"project": {"1"}, "state": {"unresolved"}, "sort": {"last_seen"}, "order": {"desc"}, "q": {q}}
	if path != "/api/canonical/0/issues/" || !reflect.DeepEqual(got, want) {
		t.Fatalf("path %q query %v, want %v", path, got, want)
	}

	_, err = c.ListIssues(context.Background(), IssueFilter{Sort: SortEventCount, Order: Asc}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want = url.Values{"sort": {"digested_event_count"}, "order": {"asc"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query %v, want %v", got, want)
	}
}

const realIssue = `{"id":"fe36e79c-e229-428a-9650-c5f31ad3fdc1","friendly_id":"SAMPLE-PYTHON-2","project":1,` +
	`"digest_order":2,"last_seen":"2026-11-15T04:47:36.852841Z","first_seen":"2026-11-13T23:06:16.473890Z",` +
	`"digested_event_count":18,"stored_event_count":18,"calculated_type":"LookupError",` +
	`"calculated_value":"failure outside any span","transaction":"/no-trace","is_resolved":false,` +
	`"is_resolved_unconditionally":false,"is_resolved_by_next_release":false,"fixed_at":[],` +
	`"events_at":["sample-python@0.1.0"],"is_muted":false,"unmute_after":null,` +
	`"unmute_on_volume_based_conditions":[]}`

func TestListIssuesMapsFields(t *testing.T) {
	flag := func(key string) string {
		return strings.Replace(realIssue, `"`+key+`":false`, `"`+key+`":true`, 1)
	}
	resolved := flag("is_resolved")
	byRelease := flag("is_resolved_by_next_release")
	muted := strings.Replace(flag("is_muted"), `"unmute_after":null`, `"unmute_after":"2026-12-06T07:08:09.123456Z"`, 1)
	body := page("", realIssue, resolved, byRelease, muted)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) })
	list, err := c.ListIssues(context.Background(), IssueFilter{Sort: SortLastSeen, Order: Desc}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Issues) != 4 {
		t.Fatalf("issues = %v", list.Issues)
	}
	want := Issue{
		ID: "fe36e79c-e229-428a-9650-c5f31ad3fdc1", FriendlyID: "SAMPLE-PYTHON-2", ProjectID: 1,
		Type: "LookupError", Value: "failure outside any span", Transaction: "/no-trace",
		FirstSeen:          time.Date(2026, 11, 13, 23, 6, 16, 473890000, time.UTC),
		LastSeen:           time.Date(2026, 11, 15, 4, 47, 36, 852841000, time.UTC),
		DigestedEventCount: 18, StoredEventCount: 18,
	}
	if !reflect.DeepEqual(list.Issues[0], want) {
		t.Fatalf("got %+v\nwant %+v", list.Issues[0], want)
	}
	flags := func(i Issue) [3]bool { return [3]bool{i.IsResolved, i.IsResolvedByNextRelease, i.IsMuted} }
	wantFlags := [][3]bool{{false, false, false}, {true, false, false}, {false, true, false}, {false, false, true}}
	for i, w := range wantFlags {
		if got := flags(list.Issues[i]); got != w {
			t.Errorf("issue %d flags = %v, want %v", i, got, w)
		}
	}
	unmute := time.Date(2026, 12, 6, 7, 8, 9, 123456000, time.UTC)
	if list.Issues[0].UnmuteAfter != nil {
		t.Fatalf("UnmuteAfter = %v, want nil", list.Issues[0].UnmuteAfter)
	}
	if u := list.Issues[3].UnmuteAfter; u == nil || !u.Equal(unmute) {
		t.Fatalf("UnmuteAfter = %v, want %v", u, unmute)
	}
}

func TestListIssuesFollowsCursor(t *testing.T) {
	var queries []url.Values
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query())
		if r.URL.Query().Get("cursor") == "c2=" {
			_, _ = fmt.Fprint(w, page("", issueJSON("P-3", "2026-01-01T00:00:00Z")))
			return
		}
		_, _ = fmt.Fprint(w, page("http://other.invalid/issues/?cursor=c2%3D",
			issueJSON("P-1", "2026-01-03T00:00:00Z"), issueJSON("P-2", "2026-01-02T00:00:00Z")))
	})
	f := IssueFilter{ProjectID: 1, State: StateOpen, Query: "x", Sort: SortLastSeen, Order: Desc}
	list, err := c.ListIssues(context.Background(), f, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, i := range list.Issues {
		ids = append(ids, i.FriendlyID)
	}
	if !reflect.DeepEqual(ids, []string{"P-1", "P-2", "P-3"}) || list.Truncated {
		t.Fatalf("ids = %v truncated = %v", ids, list.Truncated)
	}
	if len(queries) != 2 {
		t.Fatalf("requests = %d", len(queries))
	}
	want := url.Values{"project": {"1"}, "state": {"open"}, "q": {"x"}, "sort": {"last_seen"}, "order": {"desc"}}
	if queries[0].Has("cursor") {
		t.Fatalf("first request has a cursor: %v", queries[0])
	}
	want["cursor"] = []string{"c2="}
	if !reflect.DeepEqual(queries[1], want) {
		t.Fatalf("second query %v, want %v", queries[1], want)
	}
}

func TestListIssuesStopsAtSeenSince(t *testing.T) {
	requests := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = fmt.Fprint(w, page("http://x/issues/?cursor=more",
			issueJSON("P-1", "2026-01-05T00:00:00Z"), issueJSON("P-2", "2026-01-03T00:00:00Z"),
			issueJSON("P-3", "2026-01-01T00:00:00Z")))
	})
	since := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	list, err := c.ListIssues(context.Background(), IssueFilter{Sort: SortLastSeen, Order: Desc}, since)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(list.Issues) != 2 || list.Issues[1].FriendlyID != "P-2" || list.Truncated {
		t.Fatalf("requests %d issues %v truncated %v", requests, list.Issues, list.Truncated)
	}
	if !list.SeenSince.Equal(since) {
		t.Fatalf("SeenSince = %v", list.SeenSince)
	}
}

func TestListIssuesReadsEverythingForOtherSorts(t *testing.T) {
	for _, f := range []IssueFilter{{Sort: SortEventCount, Order: Desc}, {Sort: SortLastSeen, Order: Asc}} {
		requests := 0
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.URL.Query().Get("cursor") == "p2" {
				_, _ = fmt.Fprint(w, page("", issueJSON("P-3", "2026-01-01T00:00:00Z")))
				return
			}
			_, _ = fmt.Fprint(w, page("http://x/issues/?cursor=p2",
				issueJSON("P-1", "2026-01-05T00:00:00Z"), issueJSON("P-2", "2025-01-01T00:00:00Z")))
		})
		since := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
		list, err := c.ListIssues(context.Background(), f, since)
		if err != nil {
			t.Fatal(err)
		}
		if requests != 2 || len(list.Issues) != 3 || !list.SeenSince.IsZero() {
			t.Fatalf("%+v: requests %d issues %d SeenSince %v", f, requests, len(list.Issues), list.SeenSince)
		}
	}
}

func TestListIssuesStopsAtMaxPages(t *testing.T) {
	requests := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = fmt.Fprint(w, page(fmt.Sprintf("http://x/issues/?cursor=c%d", requests),
			issueJSON(fmt.Sprintf("P-%d", requests), "2026-01-01T00:00:00Z")))
	})
	list, err := c.ListIssues(context.Background(), IssueFilter{Sort: SortLastSeen, Order: Desc}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if requests != MaxPages || !list.Truncated || len(list.Issues) != MaxPages {
		t.Fatalf("requests %d truncated %v issues %d", requests, list.Truncated, len(list.Issues))
	}
}

func TestListIssuesRepeatedCursor(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, page("http://x/issues/?cursor=same", issueJSON("P-1", "2026-01-01T00:00:00Z")))
	})
	_, err := c.ListIssues(context.Background(), IssueFilter{Sort: SortLastSeen, Order: Desc}, time.Time{})
	if !errors.Is(err, ErrNotBugsink) {
		t.Fatalf("error = %v, want ErrNotBugsink", err)
	}
}

func TestBaseURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://h:8000/api/canonical/0/": "http://h:8000",
		"http://h:8000":                  "http://h:8000",
		"https://h/bugsink/":             "https://h/bugsink",
	} {
		c, err := New(in, "t", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.BaseURL(); got != want {
			t.Errorf("BaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}
