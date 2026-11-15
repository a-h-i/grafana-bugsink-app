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
	filter, invalid := parseQuery(q)
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
	frame := issuesFrame(issues, d.projectNames(ctx), d.browserURL, list.Truncated)
	frame.RefID = q.RefID
	return backend.DataResponse{Frames: data.Frames{frame}}
}

func failure(status backend.Status, message string) backend.DataResponse {
	return backend.ErrDataResponseWithSource(status, backend.ErrorSourceDownstream, message)
}

// parseQuery validates the query model and applies its defaults; invalid is the message for a bad query, or empty.
func parseQuery(q backend.DataQuery) (f bugsink.IssueFilter, invalid string) {
	var m queryModel
	if len(q.JSON) > 0 {
		if err := json.Unmarshal(q.JSON, &m); err != nil {
			return f, "The query is not valid JSON."
		}
	}
	project, err := parseProject(m.Project)
	if err != nil {
		return f, "The project must be a number."
	}
	if m.QueryType == "" {
		m.QueryType = q.QueryType
	}
	for _, c := range []struct {
		key, value string
		allowed    []string
	}{
		{"queryType", m.QueryType, []string{queryTypeIssues}},
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

// issueList returns the issues for the filter, from the cache when its list reaches back to from.
func (d *Datasource) issueList(ctx context.Context, f bugsink.IssueFilter, from time.Time) (bugsink.IssueList, error) {
	if cached, ok := d.issueCache.get(filterKey(f)); ok && (cached.SeenSince.IsZero() || !cached.SeenSince.After(from)) {
		return cached, nil
	}
	list, err := d.client.ListIssues(ctx, f, from)
	if err != nil {
		return bugsink.IssueList{}, err
	}
	if !list.Truncated {
		d.issueCache.set(filterKey(f), list)
	}
	return list, nil
}

func filterKey(f bugsink.IssueFilter) string {
	return fmt.Sprintf("%d|%s|%q|%s|%s", f.ProjectID, f.State, f.Query, f.Sort, f.Order)
}

// projects returns the project list through the cache.
func (d *Datasource) projects(ctx context.Context) ([]bugsink.Project, error) {
	if cached, ok := d.projectCache.get(""); ok {
		return cached, nil
	}
	projects, err := d.client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	d.projectCache.set("", projects)
	return projects, nil
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
