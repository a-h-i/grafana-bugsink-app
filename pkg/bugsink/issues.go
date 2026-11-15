// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink/internal/gen"
)

type IssueState string

const (
	StateAll        IssueState = "all"
	StateOpen       IssueState = "open"
	StateUnresolved IssueState = "unresolved"
	StateResolved   IssueState = "resolved"
	StateMuted      IssueState = "muted"
)

type IssueSort string

const (
	SortLastSeen    IssueSort = "last_seen"
	SortEventCount  IssueSort = "digested_event_count"
	SortDigestOrder IssueSort = "digest_order"
)

type Order string

const (
	Asc  Order = "asc"
	Desc Order = "desc"
)

// MaxPages is the most pages one list call requests.
const MaxPages = 20

type IssueFilter struct {
	ProjectID int // 0 lists every project the token can see
	State     IssueState
	Query     string
	Sort      IssueSort
	Order     Order
}

type Issue struct {
	ID, FriendlyID                       string
	ProjectID                            int
	Type, Value, Transaction             string
	FirstSeen, LastSeen                  time.Time
	DigestedEventCount, StoredEventCount int
	IsResolved, IsResolvedByNextRelease  bool
	IsMuted                              bool
	UnmuteAfter                          *time.Time
}

type IssueList struct {
	Issues    []Issue
	SeenSince time.Time // when Truncated is false, every issue last seen at or after it is in Issues; zero when the list is not bounded by time
	Truncated bool      // the page limit was reached before the list ended
}

// ListIssues pages through the issue list; sorted by last_seen descending it stops at the first issue last seen before seenSince.
func (c *Client) ListIssues(ctx context.Context, f IssueFilter, seenSince time.Time) (IssueList, error) {
	bounded := f.Sort == SortLastSeen && f.Order == Desc && !seenSince.IsZero()
	var list IssueList
	if bounded {
		list.SeenSince = seenSince
	}
	truncated, err := listPages(ctx, MaxPages, func(cursor *string) (*http.Response, error) {
		return c.gen.IssuesList(ctx, issuesParams(f, cursor))
	}, func(results []gen.Issue) bool {
		for _, r := range results {
			if bounded && r.LastSeen.Before(seenSince) {
				return true
			}
			list.Issues = append(list.Issues, issueOf(r))
		}
		return false
	})
	if err != nil {
		return IssueList{}, err
	}
	list.Truncated = truncated
	return list, nil
}

// GetIssue reads one issue by its uuid or short id.
func (c *Client) GetIssue(ctx context.Context, id string) (Issue, error) {
	resp, err := c.gen.IssuesRetrieve(ctx, id)
	if err != nil {
		return Issue{}, err
	}
	var body gen.Issue
	if err := decodeJSON(resp, &body); err != nil {
		return Issue{}, err
	}
	return issueOf(body), nil
}

// FirstIssues reads one page of issues and returns the first n; more is true when the page held more than n or had a next page.
func (c *Client) FirstIssues(ctx context.Context, f IssueFilter, n int) (issues []Issue, more bool, err error) {
	resp, err := c.gen.IssuesList(ctx, issuesParams(f, nil))
	if err != nil {
		return nil, false, err
	}
	var body pageBody[gen.Issue]
	if err := decodeJSON(resp, &body); err != nil {
		return nil, false, err
	}
	if body.Results == nil {
		return nil, false, fmt.Errorf("%w: %w", ErrNotBugsink, &APIError{Status: resp.StatusCode})
	}
	more = len(body.Results) > n || (body.Next != nil && *body.Next != "")
	for _, r := range body.Results[:min(n, len(body.Results))] {
		issues = append(issues, issueOf(r))
	}
	return issues, more, nil
}

func issuesParams(f IssueFilter, cursor *string) *gen.IssuesListParams {
	sort := gen.IssuesListParamsSort(f.Sort)
	order := gen.IssuesListParamsOrder(f.Order)
	p := &gen.IssuesListParams{Cursor: cursor, Sort: &sort, Order: &order}
	if f.ProjectID != 0 {
		p.Project = &f.ProjectID
	}
	if f.State != "" {
		state := gen.IssuesListParamsState(f.State)
		p.State = &state
	}
	if f.Query != "" {
		p.Q = &f.Query
	}
	return p
}

func issueOf(r gen.Issue) Issue {
	i := Issue{
		FriendlyID:              deref(r.FriendlyId),
		ProjectID:               r.Project,
		Type:                    deref(r.CalculatedType),
		Value:                   deref(r.CalculatedValue),
		Transaction:             deref(r.Transaction),
		FirstSeen:               r.FirstSeen,
		LastSeen:                r.LastSeen,
		DigestedEventCount:      r.DigestedEventCount,
		StoredEventCount:        deref(r.StoredEventCount),
		IsResolved:              deref(r.IsResolved),
		IsResolvedByNextRelease: deref(r.IsResolvedByNextRelease),
		IsMuted:                 deref(r.IsMuted),
		UnmuteAfter:             r.UnmuteAfter,
	}
	if r.Id != nil {
		i.ID = r.Id.String()
	}
	return i
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
