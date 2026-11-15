// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	queryTypeIssueCount = "issueCount"
	queryTypeEventCount = "eventCount"
	maxCountMinutes     = 1440
	maxCountHours       = 24

	issueCountName = "Issues"
	eventCountName = "Events"

	minutesMessage       = "The minutes must be a whole number from 1 to 1440."
	hoursMessage         = "The hours must be a whole number from 1 to 24."
	issueCountCapMessage = "More than 5000 issues match, so they cannot be counted. Narrow the query with a project, a state or a search."
	eventCountNamesNone  = "An event count needs a project or an issue."
)

// wholeNumber reads an optional JSON integer from lo to hi; absent or null gives 0, true.
func wholeNumber(raw json.RawMessage, lo, hi int) (int, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, true
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}

// issueCountQuery is a validated issue count query.
type issueCountQuery struct {
	Filter  bugsink.IssueFilter
	Minutes int // 0: any time
}

// parseIssueCountQuery validates the issue count fields of the query; invalid is the message for a bad query, or empty.
func parseIssueCountQuery(m queryModel, project int) (q issueCountQuery, invalid string) {
	m.Sort, m.Order = string(bugsink.SortLastSeen), string(bugsink.Desc)
	filter, invalid := issuesFilter(m, project)
	if invalid != "" {
		return q, invalid
	}
	minutes, ok := wholeNumber(m.Minutes, 1, maxCountMinutes)
	if !ok {
		return q, minutesMessage
	}
	return issueCountQuery{Filter: filter, Minutes: minutes}, ""
}

// queryIssueCount answers how many issues match the filter, and with minutes were last seen within them.
func (d *Datasource) queryIssueCount(ctx context.Context, m queryModel, project int) backend.DataResponse {
	q, invalid := parseIssueCountQuery(m, project)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	var seenSince time.Time
	if q.Minutes > 0 {
		seenSince = d.now().Add(-time.Duration(q.Minutes) * time.Minute)
	}
	list, err := d.issueList(ctx, q.Filter, seenSince)
	if err != nil {
		message, status := userMessage(err)
		return failure(status, message)
	}
	if list.Truncated {
		return failure(backend.StatusBadRequest, issueCountCapMessage)
	}
	count := int64(0)
	for _, i := range list.Issues {
		if !i.LastSeen.Before(seenSince) {
			count++
		}
	}
	labels := countLabels(q.Filter.ProjectID, "")
	return backend.DataResponse{Frames: data.Frames{countFrame(issueCountName, labels, []int64{count}, nil)}}
}

// eventCountQuery is a validated event count query; Issue wins over ProjectID.
type eventCountQuery struct {
	ProjectID int
	Issue     string
	Hours     int
}

// parseEventCountQuery validates the event count fields of the query; invalid is the message for a bad query, or empty.
func parseEventCountQuery(m queryModel, project int) (q eventCountQuery, invalid string) {
	if m.Issue != "" && !validIssueID(m.Issue) {
		return q, fmt.Sprintf("%q is not an issue id.", m.Issue)
	}
	hours, ok := wholeNumber(m.Hours, 1, maxCountHours)
	if !ok {
		return q, hoursMessage
	}
	if hours == 0 {
		hours = 1
	}
	return eventCountQuery{ProjectID: project, Issue: m.Issue, Hours: hours}, ""
}

// queryEventCount answers the events of the project or issue in the last hours of Bugsink's current window.
func (d *Datasource) queryEventCount(ctx context.Context, m queryModel, project int) backend.DataResponse {
	q, invalid := parseEventCountQuery(m, project)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	labels := countLabels(q.ProjectID, q.Issue)
	if q.Issue == "" && q.ProjectID == 0 {
		notice := data.Notice{Severity: data.NoticeSeverityInfo, Text: eventCountNamesNone}
		return backend.DataResponse{Frames: data.Frames{countFrame(eventCountName, labels, nil, []data.Notice{notice})}}
	}
	counts, err := d.hourCounts(ctx, q)
	if err != nil {
		message, status := userMessage(err)
		return failure(status, message)
	}
	if len(counts) < q.Hours {
		message, status := userMessage(bugsink.ErrNotBugsink)
		return failure(status, message)
	}
	var total int64
	for _, c := range counts[len(counts)-q.Hours:] {
		total += int64(c.Count)
	}
	return backend.DataResponse{Frames: data.Frames{countFrame(eventCountName, labels, []int64{total}, nil)}}
}

// hourCounts reads Bugsink's default 24-hour window through the count cache, storing it only when its last bucket is Grafana's current hour.
func (d *Datasource) hourCounts(ctx context.Context, q eventCountQuery) ([]bugsink.EventCount, error) {
	kind, id := "project", strconv.Itoa(q.ProjectID)
	if q.Issue != "" {
		kind, id = "issue", q.Issue
	}
	hour := d.now().UTC().Truncate(time.Hour)
	key := fmt.Sprintf("%s|%s|%d|%d", kind, id, q.Hours, hour.Unix())
	if cached, ok := d.countCache.get(key); ok {
		return cached, nil
	}
	return d.countFlight.do(ctx, key, func(ctx context.Context) ([]bugsink.EventCount, error) {
		var (
			counts []bugsink.EventCount
			err    error
		)
		if q.Issue != "" {
			counts, err = d.client.IssueEventCounts(ctx, q.Issue, time.Time{}, time.Time{})
		} else {
			counts, err = d.client.ProjectEventCounts(ctx, q.ProjectID, time.Time{}, time.Time{})
		}
		if err != nil {
			if errors.Is(err, bugsink.ErrNotBugsink) {
				backend.Logger.FromContext(ctx).Debug("Event counts refused", "reason", err.Error())
			}
			return nil, err
		}
		if n := len(counts); n > 0 && counts[n-1].Hour.Equal(hour) {
			d.countCache.set(key, counts)
		}
		return counts, nil
	})
}

// countLabels names the series by id: the issue as typed, else the project id as text, else none.
func countLabels(projectID int, issue string) data.Labels {
	switch {
	case issue != "":
		return data.Labels{"issue": issue}
	case projectID != 0:
		return data.Labels{"project_id": strconv.Itoa(projectID)}
	}
	return nil
}

// countFrame builds a current-value frame with zero or one value.
func countFrame(name string, labels data.Labels, values []int64, notices []data.Notice) *data.Frame {
	field := data.NewField("count", labels, orEmpty(values))
	field.Config = &data.FieldConfig{DisplayNameFromDS: name}
	frame := data.NewFrame(name, field)
	frame.Meta = &data.FrameMeta{Type: data.FrameTypeNumericMulti, TypeVersion: data.FrameTypeVersion{0, 1}}
	frame.AppendNotices(notices...)
	return frame
}
