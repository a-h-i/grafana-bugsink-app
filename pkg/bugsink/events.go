// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"net/http"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink/internal/gen"
	"github.com/google/uuid"
)

type Event struct {
	ID, EventID, IssueID  string
	ProjectID             int
	Timestamp, IngestedAt time.Time
}

type EventList struct {
	Events    []Event
	Truncated bool // maxPages was reached before the list ended
}

// ListEvents reads the events of an issue newest first, filtered by query when it is not empty, up to maxPages pages.
func (c *Client) ListEvents(ctx context.Context, issueID, query string, maxPages int) (EventList, error) {
	order := gen.EventsListParamsOrderDesc
	var list EventList
	truncated, err := listPages(ctx, maxPages, func(cursor *string) (*http.Response, error) {
		p := &gen.EventsListParams{Cursor: cursor, Issue: issueID, Order: &order}
		if query != "" {
			p.Q = &query
		}
		return c.gen.EventsList(ctx, p)
	}, func(results []gen.EventList) bool {
		for _, r := range results {
			list.Events = append(list.Events, eventOf(r))
		}
		return false
	})
	if err != nil {
		return EventList{}, err
	}
	list.Truncated = truncated
	return list, nil
}

// LookupEvent finds an event of a project by the event id its SDK sent and parses its payload.
func (c *Client) LookupEvent(ctx context.Context, projectID int, eventID uuid.UUID) (EventDetail, error) {
	resp, err := c.gen.EventsLookupRetrieve(ctx, &gen.EventsLookupRetrieveParams{EventId: eventID, Project: projectID})
	if err != nil {
		return EventDetail{}, err
	}
	return decodeEventDetail(resp)
}

func eventOf(r gen.EventList) Event {
	e := Event{ProjectID: r.Project, IssueID: r.Issue.String(), Timestamp: r.Timestamp, IngestedAt: r.IngestedAt}
	if r.Id != nil {
		e.ID = r.Id.String()
	}
	if r.EventId != nil {
		e.EventID = r.EventId.String()
	}
	return e
}
