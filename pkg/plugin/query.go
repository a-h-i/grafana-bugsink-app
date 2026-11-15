// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const queryTypeIssues = "issues"

// queryModel is the JSON of one query as saved in a dashboard.
type queryModel struct {
	QueryType string          `json:"queryType"`
	Project   json.RawMessage `json:"project"`
	State     string          `json:"state"`
	Q         string          `json:"q"`
	Sort      string          `json:"sort"`
	Order     string          `json:"order"`
	Issue     string          `json:"issue"`
	EventID   string          `json:"eventId"`
}

// QueryData answers every query of the request on its own.
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	resp := backend.NewQueryDataResponse()
	for _, q := range req.Queries {
		resp.Responses[q.RefID] = d.query(ctx, q)
	}
	return resp, nil
}

func (d *Datasource) query(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	if d.settingsErr != "" {
		return failure(backend.StatusBadRequest, d.settingsErr)
	}
	m, project, invalid := parseModel(q)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	var resp backend.DataResponse
	switch m.QueryType {
	case queryTypeEvents:
		resp = d.queryEvents(ctx, m, project)
	case queryTypeVolume:
		resp = d.queryVolume(ctx, q, m, project)
	default:
		resp = d.queryIssues(ctx, q, m, project)
	}
	for _, frame := range resp.Frames {
		frame.RefID = q.RefID
	}
	return resp
}

func (d *Datasource) queryIssues(ctx context.Context, q backend.DataQuery, m queryModel, project int) backend.DataResponse {
	filter, invalid := issuesFilter(m, project)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	from, to := q.TimeRange.From, q.TimeRange.To
	list, err := d.issueList(ctx, filter, from)
	if err != nil {
		message, status := userMessage(err)
		return failure(status, message)
	}
	issues := make([]bugsink.Issue, 0, len(list.Issues))
	for _, i := range list.Issues {
		if !i.LastSeen.Before(from) && !i.LastSeen.After(to) {
			issues = append(issues, i)
		}
	}
	return backend.DataResponse{Frames: data.Frames{issuesFrame(issues, d.projectNames(ctx), d.browserURL, list.Truncated)}}
}

func (d *Datasource) queryEvents(ctx context.Context, m queryModel, project int) backend.DataResponse {
	eq, invalid := parseEventsQuery(m, project)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	if eq.namesNothing() {
		frame := eventsFrame(nil, nil, d.browserURL, nil, d.links)
		frame.AppendNotices(data.Notice{Severity: data.NoticeSeverityInfo, Text: eventsNamesNothing})
		return backend.DataResponse{Frames: data.Frames{frame}}
	}
	res, err := d.events(ctx, eq)
	if err != nil {
		message, status := userMessage(err)
		return failure(status, message)
	}
	return backend.DataResponse{Frames: data.Frames{eventsFrame(res.rows, d.projectNames(ctx), d.browserURL, res.notices, d.links)}}
}

func failure(status backend.Status, message string) backend.DataResponse {
	return backend.ErrDataResponseWithSource(status, backend.ErrorSourceDownstream, message)
}

// parseModel reads the query model and its project and checks the query type; invalid is the message for a bad query, or empty.
func parseModel(q backend.DataQuery) (m queryModel, project int, invalid string) {
	if len(q.JSON) > 0 {
		if err := json.Unmarshal(q.JSON, &m); err != nil {
			return m, 0, "The query is not valid JSON."
		}
	}
	project, err := parseProject(m.Project)
	if err != nil {
		return m, 0, "The project must be a number."
	}
	if m.QueryType == "" {
		m.QueryType = q.QueryType
	}
	if m.QueryType != "" && !slices.Contains([]string{queryTypeIssues, queryTypeEvents, queryTypeVolume}, m.QueryType) {
		return m, 0, fmt.Sprintf("Unknown queryType %q.", m.QueryType)
	}
	return m, project, ""
}

// issuesFilter validates the issues fields of the query and applies their defaults; invalid is the message for a bad query, or empty.
func issuesFilter(m queryModel, project int) (f bugsink.IssueFilter, invalid string) {
	for _, c := range []struct {
		key, value string
		allowed    []string
	}{
		{"state", m.State, []string{
			string(bugsink.StateAll), string(bugsink.StateOpen), string(bugsink.StateUnresolved),
			string(bugsink.StateResolved), string(bugsink.StateMuted)}},
		{"sort", m.Sort, []string{
			string(bugsink.SortLastSeen), string(bugsink.SortEventCount), string(bugsink.SortDigestOrder)}},
		{"order", m.Order, []string{string(bugsink.Asc), string(bugsink.Desc)}},
	} {
		if c.value != "" && !slices.Contains(c.allowed, c.value) {
			return f, fmt.Sprintf("Unknown %s %q.", c.key, c.value)
		}
	}
	f = bugsink.IssueFilter{
		ProjectID: project,
		State:     bugsink.IssueState(m.State),
		Query:     m.Q,
		Sort:      bugsink.IssueSort(m.Sort),
		Order:     bugsink.Order(m.Order),
	}
	if f.State == "" {
		f.State = bugsink.StateUnresolved
	}
	if f.Sort == "" {
		f.Sort = bugsink.SortLastSeen
	}
	if f.Order == "" {
		f.Order = bugsink.Desc
	}
	return f, ""
}

// parseProject reads the project id; an absent or null project is 0, all projects.
func parseProject(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	return strconv.Atoi(string(raw))
}

// issueList returns the issues for the filter, from the cache when its list reaches back to from; a list read across a clear is not stored.
func (d *Datasource) issueList(ctx context.Context, f bugsink.IssueFilter, from time.Time) (bugsink.IssueList, error) {
	if cached, ok := d.issueCache.get(filterKey(f)); ok && (cached.SeenSince.IsZero() || !cached.SeenSince.After(from)) {
		return cached, nil
	}
	gen := d.issueCache.generation()
	list, err := d.client.ListIssues(ctx, f, from)
	if err != nil {
		return bugsink.IssueList{}, err
	}
	if !list.Truncated {
		d.issueCache.setIfGeneration(filterKey(f), list, gen)
	}
	return list, nil
}

func filterKey(f bugsink.IssueFilter) string {
	return fmt.Sprintf("%d|%s|%q|%s|%s", f.ProjectID, f.State, f.Query, f.Sort, f.Order)
}

// projectsResult is a project read with its error, kept in the cache.
type projectsResult struct {
	projects []bugsink.Project
	err      error
}

// projects returns the project list through the cache; a failed read is cached too, unless it timed out or was cancelled.
func (d *Datasource) projects(ctx context.Context) ([]bugsink.Project, error) {
	if cached, ok := d.projectCache.get(""); ok {
		return cached.projects, cached.err
	}
	projects, err := d.client.ListProjects(ctx)
	if isTimeout(err) {
		return nil, err
	}
	d.projectCache.set("", projectsResult{projects: projects, err: err})
	return projects, err
}

// projectNames maps project ids to names; it is empty when the project list cannot be read.
func (d *Datasource) projectNames(ctx context.Context) map[int]string {
	projects, _ := d.projects(ctx)
	names := make(map[int]string, len(projects))
	for _, p := range projects {
		names[p.ID] = p.Name
	}
	return names
}
