// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func tagJSON(id int, key, value string, count int) string {
	return fmt.Sprintf(`{"id":%d,"key":%q,"value":%q,"count":%d}`, id, key, value, count)
}

func TestListIssueTags(t *testing.T) {
	var paths []string
	var cursors []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		if r.URL.Query().Get("cursor") == "p2" {
			_, _ = fmt.Fprint(w, page("", tagJSON(3, "release", "1.0", 1)))
			return
		}
		_, _ = fmt.Fprint(w, page("http://x/tags/?cursor=p2", tagJSON(1, "environment", "development", 26), tagJSON(2, "environment", "staging", 3)))
	})
	list, err := c.ListIssueTags(context.Background(), "SAMPLE-PYTHON-2")
	if err != nil {
		t.Fatal(err)
	}
	want := []IssueTag{{"environment", "development", 26}, {"environment", "staging", 3}, {"release", "1.0", 1}}
	if !reflect.DeepEqual(list.Tags, want) || list.Truncated {
		t.Fatalf("tags %v truncated %v", list.Tags, list.Truncated)
	}
	if !reflect.DeepEqual(cursors, []string{"", "p2"}) || paths[0] != "/api/canonical/0/issues/SAMPLE-PYTHON-2/tags/" {
		t.Fatalf("paths %v cursors %v", paths, cursors)
	}
}

func TestListIssueTagsStopsAtMaxPages(t *testing.T) {
	requests := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = fmt.Fprint(w, page(fmt.Sprintf("http://x/tags/?cursor=c%d", requests), tagJSON(requests, "k", "v", 1)))
	})
	list, err := c.ListIssueTags(context.Background(), "i")
	if err != nil {
		t.Fatal(err)
	}
	if requests != MaxPages || !list.Truncated || len(list.Tags) != MaxPages {
		t.Fatalf("requests %d truncated %v tags %d", requests, list.Truncated, len(list.Tags))
	}
}

func TestListIssueHistory(t *testing.T) {
	var query url.Values
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = fmt.Fprint(w, page("",
			`{"id":277,"kind":"manual_annotation","timestamp":"2026-11-15T05:00:00Z","comment":"looked at it","user":3,"user_display_name":"Ada"}`,
			`{"id":276,"kind":"muted","timestamp":"2026-11-15T04:00:00Z","comment":"","user":null,"user_display_name":null,"metadata":{"mute_unconditionally":true}}`))
	})
	h, err := c.ListIssueHistory(context.Background(), "SAMPLE-PYTHON-264")
	if err != nil {
		t.Fatal(err)
	}
	want := []TurningPoint{
		{ID: 277, Kind: "manual_annotation", Timestamp: time.Date(2026, 11, 15, 5, 0, 0, 0, time.UTC), User: "Ada", Comment: "looked at it"},
		{ID: 276, Kind: "muted", Timestamp: time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)},
	}
	if !reflect.DeepEqual(h.Entries, want) || h.Truncated {
		t.Fatalf("entries %+v truncated %v", h.Entries, h.Truncated)
	}
	if query.Get("issue") != "SAMPLE-PYTHON-264" {
		t.Fatalf("query %v", query)
	}
}

func TestHistoryDetails(t *testing.T) {
	unmute := time.Date(2026, 11, 17, 10, 0, 0, 123456000, time.UTC)
	long := strings.Repeat("x", 2000)
	wide := strings.Repeat("é", 2000)
	cases := []struct {
		metadata string
		want     TurningPointDetails
	}{
		{`{"mute_for":{"period_name":"day","nr_of_periods":2,"unmute_after":"2026-11-17T10:00:00.123456+00:00"}}`, TurningPointDetails{Period: PeriodDay, Periods: 2, UnmuteAfter: &unmute}},
		{`{"mute_until":{"period_name":"hour","nr_of_periods":1,"gte_threshold":"5"}}`, TurningPointDetails{Period: PeriodHour, Periods: 1, Threshold: 5}},
		{`{"resolved_release":"m5-probe@0"}`, TurningPointDetails{Release: "m5-probe@0"}},
		{`{"resolve_by_next":true}`, TurningPointDetails{NextRelease: true}},
		{`{"resolved_unconditionally":true}`, TurningPointDetails{}},
		{`{"mute_unconditionally":true}`, TurningPointDetails{}},
		{`{}`, TurningPointDetails{}},
		{`{"mute_until":{"period":"hour","nr_of_periods":1,"volume":7}}`, TurningPointDetails{Period: PeriodHour, Periods: 1, Threshold: 7}},
		{`{"mute_for":{"unmute_after":"2026-11-17T12:00:00.123456+02:00"}}`, TurningPointDetails{UnmuteAfter: &unmute}},
		{`{"actual_release":"1.2.0"}`, TurningPointDetails{Release: "1.2.0"}},
		{`{"mute_until":{"period_name":"week","nr_of_periods":"3","gte_threshold":10}}`, TurningPointDetails{Period: PeriodWeek, Periods: 3, Threshold: 10}},
		{`{"resolved_release":5}`, TurningPointDetails{}},
		{`{"mute_for":"x"}`, TurningPointDetails{}},
		{`{"mute_until":{"period_name":"fortnight"}}`, TurningPointDetails{}},
		{`{"mute_until":{"period_name":"day","nr_of_periods":2}}`, TurningPointDetails{}},
		{`null`, TurningPointDetails{}},
		{`"resolved"`, TurningPointDetails{}},
		{`[{"resolved_release":"1"}]`, TurningPointDetails{}},
		{`{"resolve_by_next":"true"}`, TurningPointDetails{}},
		{`{"mute_for":{"period_name":"Day","nr_of_periods":1.5,"unmute_after":"tomorrow"}}`, TurningPointDetails{}},
		{`{"mute_until":{"period_name":"day","nr_of_periods":-1,"gte_threshold":"1e3"}}`, TurningPointDetails{}},
		{`{"mute_until":{"period_name":"day","nr_of_periods":1e20,"gte_threshold":"99999999999999999999"}}`, TurningPointDetails{}},
		{`{"mute_until":{"period_name":"day","nr_of_periods":0,"gte_threshold":" 5"}}`, TurningPointDetails{}},
		{`{"resolved_release":"` + long + `"}`, TurningPointDetails{Release: long[:1024]}},
		{`{"resolved_release":"` + wide + `"}`, TurningPointDetails{Release: strings.Repeat("é", 1024)}},
	}
	var entries []string
	for i, c := range cases {
		entries = append(entries, fmt.Sprintf(`{"id":%d,"kind":"muted","timestamp":"2026-11-15T04:00:00Z","metadata":%s}`, i, c.metadata))
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, page("", entries...))
	})
	h, err := client.ListIssueHistory(context.Background(), "SAMPLE-PYTHON-264")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Entries) != len(cases) {
		t.Fatalf("%d entries, want %d", len(h.Entries), len(cases))
	}
	for i, c := range cases {
		if got := h.Entries[i].Details; !reflect.DeepEqual(got, c.want) {
			t.Errorf("metadata %.80s: details %+v, want %+v", c.metadata, got, c.want)
		}
	}
}
