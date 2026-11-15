// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	queryTypeNewIssues = "newIssues"
	maxNewIssuePoints  = 1440
	maxNewIssueStep    = 100 * 365 * 24 * time.Hour

	newIssuesName       = "New issues"
	newIssuesEmptyRange = "The time range must end after it starts."
	newIssuesCapMessage = "More than 5000 issues were seen in the time range, so the new ones cannot be counted. Narrow the query with a project or a search, or shorten the time range."
	newIssuesNoFirst    = "Bugsink sent an issue without a valid first seen time, so the new issues cannot be counted."
)

// stepCount is the number of steps of stepSeconds from the aligned start that begin before to.
func stepCount(start, to time.Time, stepSeconds int64) int64 {
	last := to.Unix()
	if to.Nanosecond() == 0 {
		last--
	}
	if last < start.Unix() {
		return 0
	}
	return (last-start.Unix())/stepSeconds + 1
}

// newIssueStep is interval rounded up to whole minutes, at least one minute and at least the range over maxNewIssuePoints.
func newIssueStep(from, to time.Time, interval time.Duration) time.Duration {
	interval = max(min(interval, maxNewIssueStep), time.Minute)
	seconds := int64(interval / time.Second)
	if interval%time.Second > 0 {
		seconds++
	}
	if rangeSeconds := (to.Unix() - from.Unix()) / maxNewIssuePoints; rangeSeconds > seconds {
		seconds = rangeSeconds
	}
	for seconds%60 != 0 {
		seconds++
	}
	return time.Duration(seconds) * time.Second
}

// newIssueWindow is the epoch-aligned start and the step of the window; the step grows until the window holds at most maxNewIssuePoints steps.
func newIssueWindow(from, to time.Time, interval time.Duration) (start time.Time, step time.Duration) {
	step = newIssueStep(from, to, interval)
	for {
		start = alignDown(from, int64(step/time.Second))
		if stepCount(start, to, int64(step/time.Second)) <= maxNewIssuePoints {
			return start, step
		}
		step += time.Minute
	}
}

// newIssueSeries counts issues by first_seen into epoch-aligned steps from start that begin before to; first_seen in [start, to).
func newIssueSeries(issues []bugsink.Issue, start, to time.Time, step time.Duration) (starts []time.Time, totals []int64) {
	stepSeconds := int64(step / time.Second)
	if stepSeconds <= 0 {
		return nil, nil
	}
	n := stepCount(start, to, stepSeconds)
	starts, totals = make([]time.Time, n), make([]int64, n)
	for i := range starts {
		starts[i] = start.Add(time.Duration(i) * step)
	}
	for _, issue := range issues {
		if issue.FirstSeen.Before(start) || !issue.FirstSeen.Before(to) {
			continue
		}
		if i := (issue.FirstSeen.Unix() - start.Unix()) / stepSeconds; i < n {
			totals[i]++
		}
	}
	return starts, totals
}

// queryNewIssues answers how many issues were first seen in each step of the time range.
func (d *Datasource) queryNewIssues(ctx context.Context, q backend.DataQuery, m queryModel, project int) backend.DataResponse {
	from, to := q.TimeRange.From, q.TimeRange.To
	if !to.After(from) {
		return failure(backend.StatusBadRequest, newIssuesEmptyRange)
	}
	m.State, m.Sort, m.Order = string(bugsink.StateAll), string(bugsink.SortLastSeen), string(bugsink.Desc)
	filter, invalid := issuesFilter(m, project)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	start, step := newIssueWindow(from, to, q.Interval)
	list, err := d.issueList(ctx, filter, start)
	if err != nil {
		message, status := userMessage(err)
		return failure(status, message)
	}
	if list.Truncated {
		return failure(backend.StatusBadRequest, newIssuesCapMessage)
	}
	for _, issue := range list.Issues {
		if issue.FirstSeen.IsZero() {
			return failure(backend.StatusBadGateway, newIssuesNoFirst)
		}
	}
	starts, totals := newIssueSeries(list.Issues, start, to, step)
	var labels data.Labels
	if project != 0 {
		labels = countLabels(project, "")
	}
	return backend.DataResponse{Frames: data.Frames{volumeFrame(newIssuesName, labels, step, starts, totals, nil)}}
}
