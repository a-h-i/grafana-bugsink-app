// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
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
