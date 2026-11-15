// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink/internal/gen"
)

// MaxEventCountBuckets is the most hourly buckets read from one answer: Bugsink's 90 days.
const MaxEventCountBuckets = 90 * 24

// maxHourlyCount is the largest hourly count read; a bigger one is not an answer from Bugsink.
const maxHourlyCount = 1_000_000_000

// EventCount is the number of events Bugsink digested in one UTC hour.
type EventCount struct {
	Hour  time.Time
	Count int
}

// ProjectEventCounts reads a project's hourly counts from start up to end; a zero end leaves end to Bugsink, the end of its current hour.
func (c *Client) ProjectEventCounts(ctx context.Context, projectID int, start, end time.Time) ([]EventCount, error) {
	p := &gen.ProjectsEventCountsRetrieveParams{Start: utcTime(start), End: utcTime(end)}
	resp, err := c.gen.ProjectsEventCountsRetrieve(ctx, projectID, p)
	if err != nil {
		return nil, err
	}
	return decodeEventCounts(resp)
}

// IssueEventCounts is ProjectEventCounts for one issue, by uuid or short id.
func (c *Client) IssueEventCounts(ctx context.Context, issueID string, start, end time.Time) ([]EventCount, error) {
	p := &gen.IssuesEventCountsRetrieveParams{Start: utcTime(start), End: utcTime(end)}
	resp, err := c.gen.IssuesEventCountsRetrieve(ctx, issueID, p)
	if err != nil {
		return nil, err
	}
	return decodeEventCounts(resp)
}

func utcTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// decodeEventCounts accepts only hourly buckets, ascending on the hour, in the bounded number and range of counts.
func decodeEventCounts(resp *http.Response) ([]EventCount, error) {
	var body gen.EventCounts
	if err := decodeJSON(resp, &body); err != nil {
		return nil, err
	}
	notBugsink := func(why string) error {
		return fmt.Errorf("%w: event counts %s: %w", ErrNotBugsink, why, &APIError{Status: resp.StatusCode})
	}
	if body.IntervalSeconds != int(time.Hour/time.Second) {
		return nil, notBugsink("are not hourly")
	}
	if body.Buckets == nil || len(body.Buckets) > MaxEventCountBuckets {
		return nil, notBugsink("have no buckets or too many")
	}
	counts := make([]EventCount, len(body.Buckets))
	for i, b := range body.Buckets {
		hour := b.Timestamp.UTC()
		if !hour.Equal(hour.Truncate(time.Hour)) || (i > 0 && !hour.After(counts[i-1].Hour)) {
			return nil, notBugsink("are not ascending on the hour")
		}
		if b.Count < 0 || b.Count > maxHourlyCount {
			return nil, notBugsink("hold an impossible count")
		}
		counts[i] = EventCount{Hour: hour, Count: b.Count}
	}
	return counts, nil
}
