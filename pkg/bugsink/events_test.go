// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	eventUUID = "0f0e0d0c-0b0a-4908-8706-050403020100"
	sdkUUID   = "426ac644-1b2a-4c3d-8e4f-0123456789ab"
	issueUUID = "fe36e79c-e229-428a-9650-c5f31ad3fdc1"
)

func eventJSON(id, eventID string) string {
	return fmt.Sprintf(`{"id":%q,"ingested_at":"2026-11-15T04:47:37Z","digested_at":"2026-11-15T04:47:38Z",`+
		`"issue":%q,"grouping":1,"event_id":%q,"project":1,"timestamp":"2026-11-15T04:47:36Z","digest_order":5}`,
		id, issueUUID, eventID)
}

func TestListEvents(t *testing.T) {
	var query url.Values
	var path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query, path = r.URL.Query(), r.URL.Path
		_, _ = fmt.Fprint(w, page("", eventJSON(eventUUID, sdkUUID)))
	})
	q := `environment:dev "a b" &x=1`
	list, err := c.ListEvents(context.Background(), "SAMPLE-PYTHON-2", q, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := Event{
		ID: eventUUID, EventID: sdkUUID, IssueID: issueUUID, ProjectID: 1,
		Timestamp:  time.Date(2026, 11, 15, 4, 47, 36, 0, time.UTC),
		IngestedAt: time.Date(2026, 11, 15, 4, 47, 37, 0, time.UTC),
	}
	if len(list.Events) != 1 || !reflect.DeepEqual(list.Events[0], want) || list.Truncated {
		t.Fatalf("events %+v truncated %v", list.Events, list.Truncated)
	}
	wantQuery := url.Values{"issue": {"SAMPLE-PYTHON-2"}, "order": {"desc"}, "q": {q}}
	if path != "/api/canonical/0/events/" || !reflect.DeepEqual(query, wantQuery) {
		t.Fatalf("path %q query %v", path, query)
	}

	if _, err := c.ListEvents(context.Background(), "SAMPLE-PYTHON-2", "", 4); err != nil {
		t.Fatal(err)
	}
	if query.Has("q") {
		t.Fatalf("q sent when empty: %v", query)
	}
}

func TestListEventsStopsAtMaxPages(t *testing.T) {
	requests := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = fmt.Fprint(w, page(fmt.Sprintf("http://x/events/?cursor=c%d", requests), eventJSON(eventUUID, sdkUUID)))
	})
	list, err := c.ListEvents(context.Background(), "i", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 4 || !list.Truncated || len(list.Events) != 4 {
		t.Fatalf("requests %d truncated %v events %d", requests, list.Truncated, len(list.Events))
	}
}

func TestLookupEvent(t *testing.T) {
	var query url.Values
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_, _ = fmt.Fprint(w, `{"id":"`+eventUUID+`","ingested_at":"2026-11-15T04:47:37Z","digested_at":"2026-11-15T04:47:38Z",`+
			`"issue":"`+issueUUID+`","grouping":1,"event_id":"`+sdkUUID+`","project":1,"timestamp":"2026-11-15T04:47:36Z",`+
			`"digest_order":5,"data":{"exception":{"values":[]}}}`)
	})
	e, err := c.LookupEvent(context.Background(), 1, uuid.MustParse(sdkUUID))
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != eventUUID || e.EventID != sdkUUID || e.IssueID != issueUUID || e.ProjectID != 1 {
		t.Fatalf("event %+v", e)
	}
	if want := (url.Values{"project": {"1"}, "event_id": {sdkUUID}}); !reflect.DeepEqual(query, want) {
		t.Fatalf("query %v", query)
	}

	notFound := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = fmt.Fprint(w, `{"detail":"No Event matches the given query."}`)
	})
	_, err = notFound.LookupEvent(context.Background(), 1, uuid.MustParse(sdkUUID))
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Detail != "No Event matches the given query." {
		t.Fatalf("error = %v", err)
	}
}

func TestLookupEventParsesPayload(t *testing.T) {
	body := fixtureBytes(t, "python-handled")
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
	e, err := c.LookupEvent(context.Background(), 1, uuid.MustParse(sdkUUID))
	if err != nil {
		t.Fatal(err)
	}
	want := getFixture(t, "python-handled")
	if e.TraceID == "" || len(e.Tags) == 0 || e.Environment == "" {
		t.Fatalf("detail = %+v", e)
	}
	if e.TraceID != want.TraceID || e.Environment != want.Environment || !reflect.DeepEqual(e.Tags, want.Tags) ||
		!reflect.DeepEqual(e.Event, want.Event) {
		t.Fatalf("lookup %+v differs from get %+v", e, want)
	}
}
