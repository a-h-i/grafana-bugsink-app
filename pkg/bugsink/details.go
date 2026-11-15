// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"net/http"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink/internal/gen"
)

type IssueTag struct {
	Key, Value string
	Count      int
}

type TagList struct {
	Tags      []IssueTag
	Truncated bool // the page limit was reached before the list ended
}

// ListIssueTags reads the tag values of an issue in Bugsink's order.
func (c *Client) ListIssueTags(ctx context.Context, issueID string) (TagList, error) {
	var list TagList
	truncated, err := listPages(ctx, MaxPages, func(cursor *string) (*http.Response, error) {
		return c.gen.IssuesTagsList(ctx, issueID, &gen.IssuesTagsListParams{Cursor: cursor})
	}, func(results []gen.IssueTag) bool {
		for _, r := range results {
			list.Tags = append(list.Tags, IssueTag{Key: deref(r.Key), Value: deref(r.Value), Count: deref(r.Count)})
		}
		return false
	})
	if err != nil {
		return TagList{}, err
	}
	list.Truncated = truncated
	return list, nil
}

type TurningPoint struct {
	ID            int
	Kind          string
	Timestamp     time.Time
	User, Comment string // User is the author's display name, empty when none is recorded
}

type History struct {
	Entries   []TurningPoint
	Truncated bool // the page limit was reached before the list ended
}

// ListIssueHistory reads the turning points of an issue, newest first.
func (c *Client) ListIssueHistory(ctx context.Context, issueID string) (History, error) {
	var h History
	truncated, err := listPages(ctx, MaxPages, func(cursor *string) (*http.Response, error) {
		return c.gen.IssueHistoryList(ctx, &gen.IssueHistoryListParams{Cursor: cursor, Issue: issueID})
	}, func(results []gen.TurningPoint) bool {
		for _, r := range results {
			h.Entries = append(h.Entries, TurningPoint{
				ID:        deref(r.Id),
				Kind:      string(deref(r.Kind)),
				Timestamp: deref(r.Timestamp),
				User:      deref(r.UserDisplayName),
				Comment:   deref(r.Comment),
			})
		}
		return false
	})
	if err != nil {
		return History{}, err
	}
	h.Truncated = truncated
	return h, nil
}
