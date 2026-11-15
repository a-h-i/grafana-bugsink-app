// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"fmt"
	"strconv"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// issuesFrame builds the frame of the frame contract.
func issuesFrame(issues []bugsink.Issue, projectNames map[int]string, browserURL string, truncated bool) *data.Frame {
	var (
		friendlyID, calcType, calcValue, transaction, project, status, id []string
		digested, stored, projectID                                       []int64
		firstSeen, lastSeen                                               []time.Time
		resolved, resolvedNext, muted                                     []bool
		unmuteAfter                                                       []*time.Time
	)
	for _, i := range issues {
		name, ok := projectNames[i.ProjectID]
		if !ok {
			name = strconv.Itoa(i.ProjectID)
		}
		friendlyID = append(friendlyID, i.FriendlyID)
		calcType = append(calcType, i.Type)
		calcValue = append(calcValue, i.Value)
		transaction = append(transaction, i.Transaction)
		project = append(project, name)
		status = append(status, statusOf(i))
		digested = append(digested, int64(i.DigestedEventCount))
		stored = append(stored, int64(i.StoredEventCount))
		firstSeen = append(firstSeen, i.FirstSeen)
		lastSeen = append(lastSeen, i.LastSeen)
		resolved = append(resolved, i.IsResolved)
		resolvedNext = append(resolvedNext, i.IsResolvedByNextRelease)
		muted = append(muted, i.IsMuted)
		unmuteAfter = append(unmuteAfter, i.UnmuteAfter)
		projectID = append(projectID, int64(i.ProjectID))
		id = append(id, i.ID)
	}

	issueField := field("friendly_id", "Issue", orEmpty(friendlyID))
	issueField.Config.Links = []data.DataLink{{
		Title:       "Open in Bugsink",
		URL:         browserURL + "/issues/issue/${__data.fields.id}/event/last/",
		TargetBlank: true,
	}}
	frame := data.NewFrame("issues",
		issueField,
		field("calculated_type", "Type", orEmpty(calcType)),
		field("calculated_value", "Message", orEmpty(calcValue)),
		field("transaction", "Transaction", orEmpty(transaction)),
		field("project", "Project", orEmpty(project)),
		field("status", "Status", orEmpty(status)),
		field("digested_event_count", "Events", orEmpty(digested)),
		field("stored_event_count", "Stored events", orEmpty(stored)),
		field("first_seen", "First seen", orEmpty(firstSeen)),
		field("last_seen", "Last seen", orEmpty(lastSeen)),
		field("is_resolved", "Resolved", orEmpty(resolved)),
		field("is_resolved_by_next_release", "Resolved by next release", orEmpty(resolvedNext)),
		field("is_muted", "Muted", orEmpty(muted)),
		field("unmute_after", "Unmute after", orEmpty(unmuteAfter)),
		field("project_id", "Project ID", orEmpty(projectID)),
		data.NewField("id", nil, orEmpty(id)),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	if truncated {
		frame.AppendNotices(data.Notice{
			Severity: data.NoticeSeverityWarning,
			Text:     fmt.Sprintf("Only the first %d issues were read. Narrow the search or the time range.", bugsink.MaxPages*bugsink.PageSize),
		})
	}
	return frame
}

// eventsFrame builds the events frame of the frame contract, with each notice as a warning.
func eventsFrame(rows []eventRow, projectNames map[int]string, browserURL string, notices []string) *data.Frame {
	var (
		eventID, issue, project, id, issueID []string
		timestamp, ingestedAt                []time.Time
		projectID                            []int64
	)
	for _, r := range rows {
		e := r.event
		name, ok := projectNames[e.ProjectID]
		if !ok {
			name = strconv.Itoa(e.ProjectID)
		}
		timestamp = append(timestamp, e.Timestamp)
		eventID = append(eventID, e.EventID)
		issue = append(issue, r.issue)
		project = append(project, name)
		ingestedAt = append(ingestedAt, e.IngestedAt)
		id = append(id, e.ID)
		issueID = append(issueID, e.IssueID)
		projectID = append(projectID, int64(e.ProjectID))
	}

	eventIDField := field("event_id", "Event ID", orEmpty(eventID))
	eventIDField.Config.Links = []data.DataLink{{
		Title:       "Open in Bugsink",
		URL:         browserURL + "/issues/issue/${__data.fields.issue_id}/event/${__data.fields.id}/",
		TargetBlank: true,
	}}
	frame := data.NewFrame("events",
		field("timestamp", "Timestamp", orEmpty(timestamp)),
		eventIDField,
		field("issue", "Issue", orEmpty(issue)),
		field("project", "Project", orEmpty(project)),
		field("ingested_at", "Ingested at", orEmpty(ingestedAt)),
		data.NewField("id", nil, orEmpty(id)),
		data.NewField("issue_id", nil, orEmpty(issueID)),
		field("project_id", "Project ID", orEmpty(projectID)),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	for _, text := range notices {
		frame.AppendNotices(data.Notice{Severity: data.NoticeSeverityWarning, Text: text})
	}
	return frame
}

func field[T any](name, display string, values []T) *data.Field {
	f := data.NewField(name, nil, values)
	f.Config = &data.FieldConfig{DisplayNameFromDS: display}
	return f
}

// orEmpty turns a nil slice into an empty one so the field keeps its type.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func statusOf(i bugsink.Issue) string {
	switch {
	case i.IsResolved:
		return "resolved"
	case i.IsMuted:
		return "muted"
	default:
		return "open"
	}
}
