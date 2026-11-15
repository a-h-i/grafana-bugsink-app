// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

const countsBody = `{"start":"2026-11-08T13:00:00Z","end":"2026-11-08T16:00:00Z","interval_seconds":3600,"buckets":[` +
	`{"timestamp":"2026-11-08T13:00:00Z","count":0},{"timestamp":"2026-11-08T14:00:00Z","count":7},` +
	`{"timestamp":"2026-11-08T15:00:00Z","count":3}]}`

func hour(day, h int) time.Time { return time.Date(2026, 11, day, h, 0, 0, 0, time.UTC) }

func countsClient(t *testing.T, body string) (*Client, *url.Values, *string) {
	t.Helper()
	var query url.Values
	var path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query, path = r.URL.Query(), r.URL.Path
		_, _ = fmt.Fprint(w, body)
	})
	return c, &query, &path
}

func TestProjectEventCounts(t *testing.T) {
	c, query, path := countsClient(t, countsBody)
	counts, err := c.ProjectEventCounts(context.Background(), 1, hour(8, 13), hour(15, 14))
	if err != nil {
		t.Fatal(err)
	}
	want := []EventCount{{hour(8, 13), 0}, {hour(8, 14), 7}, {hour(8, 15), 3}}
	if !reflect.DeepEqual(counts, want) || counts[0].Hour.Location() != time.UTC {
		t.Fatalf("counts %+v", counts)
	}
	wantQuery := url.Values{"start": {"2026-11-08T13:00:00Z"}, "end": {"2026-11-15T14:00:00Z"}}
	if *path != "/api/canonical/0/projects/1/event-counts/" || !reflect.DeepEqual(*query, wantQuery) {
		t.Fatalf("path %q query %v", *path, *query)
	}

	if _, err := c.ProjectEventCounts(context.Background(), 1, hour(8, 13), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if want := (url.Values{"start": {"2026-11-08T13:00:00Z"}}); !reflect.DeepEqual(*query, want) {
		t.Fatalf("zero end sent: %v", *query)
	}
}

func TestIssueEventCounts(t *testing.T) {
	c, _, path := countsClient(t, countsBody)
	counts, err := c.IssueEventCounts(context.Background(), "SAMPLE-PYTHON-2", hour(8, 13), hour(8, 16))
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 3 || *path != "/api/canonical/0/issues/SAMPLE-PYTHON-2/event-counts/" {
		t.Fatalf("counts %+v path %q", counts, *path)
	}
}

func TestEventCountsSendUTC(t *testing.T) {
	c, query, _ := countsClient(t, countsBody)
	plus2 := time.FixedZone("", 2*3600)
	start := time.Date(2026, 11, 8, 15, 0, 0, 0, plus2)
	if _, err := c.ProjectEventCounts(context.Background(), 1, start, time.Date(2026, 11, 8, 18, 0, 0, 0, plus2)); err != nil {
		t.Fatal(err)
	}
	want := url.Values{"start": {"2026-11-08T13:00:00Z"}, "end": {"2026-11-08T16:00:00Z"}}
	if !reflect.DeepEqual(*query, want) {
		t.Fatalf("query %v", *query)
	}
}

func TestEventCountsRefuseWhatIsNotHourly(t *testing.T) {
	bad := map[string]string{
		"daily":                 strings.Replace(countsBody, `"interval_seconds":3600`, `"interval_seconds":86400`, 1),
		"no buckets key":        `{"start":"2026-11-08T13:00:00Z","end":"2026-11-08T16:00:00Z","interval_seconds":3600}`,
		"not an object":         `[]`,
		"off the hour":          strings.Replace(countsBody, `14:00:00Z","count":7`, `14:30:00Z","count":7`, 1),
		"out of order":          strings.Replace(countsBody, `15:00:00Z","count":3`, `14:00:00Z","count":3`, 1),
		"descending":            strings.Replace(countsBody, `15:00:00Z","count":3`, `13:00:00Z","count":3`, 1),
		"count above the bound": strings.Replace(countsBody, `"count":7`, `"count":1000000001`, 1),
		"negative count":        strings.Replace(countsBody, `"count":7`, `"count":-7`, 1),
		"absurd count":          strings.Replace(countsBody, `"count":7`, `"count":9000000000000`, 1),
		"too many buckets": `{"start":"2026-01-01T00:00:00Z","end":"2026-04-12T01:00:00Z","interval_seconds":3600,"buckets":[` +
			manyBuckets(MaxEventCountBuckets+1) + `]}`,
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			c, _, _ := countsClient(t, body)
			counts, err := c.ProjectEventCounts(context.Background(), 1, hour(8, 13), hour(8, 16))
			if !errors.Is(err, ErrNotBugsink) || counts != nil {
				t.Fatalf("counts %v error %v", counts, err)
			}
		})
	}

	c, _, _ := countsClient(t, `{"interval_seconds":3600,"buckets":[`+manyBuckets(MaxEventCountBuckets)+`]}`)
	counts, err := c.ProjectEventCounts(context.Background(), 1, hour(1, 0), time.Time{})
	if err != nil || len(counts) != MaxEventCountBuckets {
		t.Fatalf("limit: %d buckets, error %v", len(counts), err)
	}
}

func TestEventCountsAcceptTheLargestCount(t *testing.T) {
	c, _, _ := countsClient(t, strings.Replace(countsBody, `"count":7`, `"count":1000000000`, 1))
	counts, err := c.ProjectEventCounts(context.Background(), 1, hour(8, 13), hour(8, 16))
	if err != nil || len(counts) != 3 || counts[1].Count != 1000000000 {
		t.Fatalf("counts %v error %v", counts, err)
	}
}

func manyBuckets(n int) string {
	parts := make([]string, n)
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"timestamp":%q,"count":1}`, t.Add(time.Duration(i)*time.Hour).Format(time.RFC3339))
	}
	return strings.Join(parts, ",")
}

func TestEventCountsRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   APIError
	}{
		"range": {400, `{"start":["The range cannot exceed 90 days."]}`,
			APIError{Status: 400, Detail: "start: The range cannot exceed 90 days."}},
		"capability": {403, `{"detail":"This token does not have the required capability: issues:read."}`,
			APIError{Status: 403, Capability: "issues:read", Detail: "This token does not have the required capability: issues:read."}},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			_, err := c.IssueEventCounts(context.Background(), "SAMPLE-PYTHON-2", hour(8, 13), hour(8, 16))
			var apiErr *APIError
			if !errors.As(err, &apiErr) || *apiErr != tc.want {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
