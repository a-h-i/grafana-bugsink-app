// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"math"
	"net/http"
	"time"
	"unicode/utf8"

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
	Details       TurningPointDetails
}

// TurningPointDetails is what a turning point's metadata says about the state change; a field Bugsink did not send validly is zero.
type TurningPointDetails struct {
	Release            string
	NextRelease        bool
	Period             Period
	Periods, Threshold int
	UnmuteAfter        *time.Time
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
				Details:   detailsOf(r.Metadata),
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

const maxReleaseRunes = 1024

// detailsOf reads the documented keys of a turning point's metadata, ignoring every other key and any value of the wrong type.
func detailsOf(metadata any) TurningPointDetails {
	m, _ := metadata.(map[string]any)
	var d TurningPointDetails
	if release, ok := m["resolved_release"].(string); ok {
		d.Release = cutRunes(release, maxReleaseRunes)
	} else if release, ok := m["actual_release"].(string); ok {
		d.Release = cutRunes(release, maxReleaseRunes)
	}
	d.NextRelease, _ = m["resolve_by_next"].(bool)
	if mute, ok := m["mute_for"].(map[string]any); ok {
		d.Period = periodOf(mute["period_name"])
		d.Periods = wholeNumber(mute["nr_of_periods"])
		d.UnmuteAfter = timestampOf(mute["unmute_after"])
	} else if mute, ok := m["mute_until"].(map[string]any); ok {
		d.Threshold = wholeNumber(mute["gte_threshold"])
		if d.Threshold == 0 {
			d.Threshold = wholeNumber(mute["volume"])
		}
		if d.Threshold > 0 {
			d.Period = periodOf(mute["period_name"])
			if d.Period == "" {
				d.Period = periodOf(mute["period"])
			}
			d.Periods = wholeNumber(mute["nr_of_periods"])
		}
	}
	return d
}

func periodOf(v any) Period {
	if p, ok := v.(string); ok && Period(p).Valid() {
		return Period(p)
	}
	return ""
}

// wholeNumber is v as a positive whole number from a JSON number or a string of digits, or 0.
func wholeNumber(v any) int {
	var n float64
	switch v := v.(type) {
	case float64:
		n = v
	case string:
		if v == "" || len(v) > 10 {
			return 0
		}
		for _, r := range v {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + float64(r-'0')
		}
	default:
		return 0
	}
	if n < 1 || n > math.MaxInt32 || n != math.Trunc(n) {
		return 0
	}
	return int(n)
}

func timestampOf(v any) *time.Time {
	s, ok := v.(string)
	if !ok || len(s) > 64 {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

// cutRunes cuts s to at most max characters.
func cutRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
