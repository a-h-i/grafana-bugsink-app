// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"fmt"
	"regexp"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/google/uuid"
)

const (
	queryTypeEvents    = "events"
	maxEventPages      = 4
	maxTraceIssues     = 10
	maxTraceEventPages = 1
)

var traceTerm = regexp.MustCompile(`(^|\s)trace:\S`)

// eventsQuery is a validated events query; the first set of EventID (uuid.Nil when absent), Issue, trace term in Query decides the mode.
type eventsQuery struct {
	ProjectID    int
	Issue, Query string
	EventID      uuid.UUID
}

type eventRow struct {
	event bugsink.Event
	issue string // the short id
}

type eventsResult struct {
	rows    []eventRow
	notices []string
}

// parseEventsQuery validates the events fields of the query; invalid is the message for a bad query, or empty.
func parseEventsQuery(m queryModel, project int) (q eventsQuery, invalid string) {
	if m.Issue != "" && !validIssueID(m.Issue) {
		return q, fmt.Sprintf("%q is not an issue id.", m.Issue)
	}
	if m.EventID != "" {
		id, err := uuid.Parse(m.EventID)
		if err != nil {
			return q, "The event ID must be a UUID."
		}
		if project == 0 {
			return q, "Looking up an event ID needs a project."
		}
		q.EventID = id
	}
	q.ProjectID, q.Issue, q.Query = project, m.Issue, m.Q
	return q, ""
}

const eventsNamesNothing = "An events query needs an issue, an event ID or trace:<id> in Search."

// namesNothing reports whether the query has no event id, issue or trace term to read.
func (q eventsQuery) namesNothing() bool {
	return q.EventID == uuid.Nil && q.Issue == "" && !traceTerm.MatchString(q.Query)
}

// key identifies the query's mode and every input that reaches Bugsink.
func (q eventsQuery) key() string {
	return fmt.Sprintf("%s|%d|%q|%q", q.EventID, q.ProjectID, q.Issue, q.Query)
}

// events reads the events the query names, through the cache; a failed read is not stored.
func (d *Datasource) events(ctx context.Context, q eventsQuery) (eventsResult, error) {
	if cached, ok := d.eventCache.get(q.key()); ok {
		return cached, nil
	}
	var (
		res eventsResult
		err error
	)
	switch {
	case q.EventID != uuid.Nil:
		res, err = d.eventByID(ctx, q)
	case q.Issue != "":
		res, err = d.eventsOfIssue(ctx, q)
	default:
		res, err = d.eventsOfTrace(ctx, q)
	}
	if err != nil {
		return eventsResult{}, err
	}
	d.eventCache.set(q.key(), res)
	return res, nil
}

func (d *Datasource) eventByID(ctx context.Context, q eventsQuery) (eventsResult, error) {
	event, err := d.client.LookupEvent(ctx, q.ProjectID, q.EventID)
	if err != nil {
		return eventsResult{}, err
	}
	issue, err := d.client.GetIssue(ctx, event.IssueID)
	if err != nil {
		return eventsResult{}, err
	}
	return eventsResult{rows: []eventRow{{event: event, issue: issue.FriendlyID}}}, nil
}

func (d *Datasource) eventsOfIssue(ctx context.Context, q eventsQuery) (eventsResult, error) {
	issue, err := d.client.GetIssue(ctx, q.Issue)
	if err != nil {
		return eventsResult{}, err
	}
	list, err := d.client.ListEvents(ctx, issue.ID, q.Query, maxEventPages)
	if err != nil {
		return eventsResult{}, err
	}
	res := eventsResult{rows: rowsOf(list.Events, issue.FriendlyID)}
	if list.Truncated {
		res.notices = append(res.notices, fmt.Sprintf("Only the newest %d events were read.", maxEventPages*bugsink.PageSize))
	}
	return res, nil
}

// eventsOfTrace reads the events matching the query of the first issues that match it, grouped by issue.
func (d *Datasource) eventsOfTrace(ctx context.Context, q eventsQuery) (eventsResult, error) {
	filter := bugsink.IssueFilter{
		ProjectID: q.ProjectID, State: bugsink.StateAll, Query: q.Query, Sort: bugsink.SortLastSeen, Order: bugsink.Desc,
	}
	issues, more, err := d.client.FirstIssues(ctx, filter, maxTraceIssues)
	if err != nil {
		return eventsResult{}, err
	}
	var res eventsResult
	if more {
		res.notices = append(res.notices,
			fmt.Sprintf("This trace has events in more than %d issues. Only the %d last seen were read.", maxTraceIssues, maxTraceIssues))
	}
	for _, issue := range issues {
		list, err := d.client.ListEvents(ctx, issue.ID, q.Query, maxTraceEventPages)
		if err != nil {
			return eventsResult{}, err
		}
		res.rows = append(res.rows, rowsOf(list.Events, issue.FriendlyID)...)
		if list.Truncated {
			res.notices = append(res.notices,
				fmt.Sprintf("Only the newest %d events of %s in this trace were read.", maxTraceEventPages*bugsink.PageSize, issue.FriendlyID))
		}
	}
	return res, nil
}

func rowsOf(events []bugsink.Event, issue string) []eventRow {
	rows := make([]eventRow, len(events))
	for i, e := range events {
		rows[i] = eventRow{event: e, issue: issue}
	}
	return rows
}
