// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"fmt"
	"net/http"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink/internal/gen"
)

// TriageAction is a state change Bugsink offers on an issue.
type TriageAction string

const (
	Resolve TriageAction = "resolve"
	Reopen  TriageAction = "reopen"
	Mute    TriageAction = "mute"
	Unmute  TriageAction = "unmute"
)

// Triage runs one bodiless triage action and returns the issue Bugsink answers with.
func (c *Client) Triage(ctx context.Context, issueID string, action TriageAction) (Issue, error) {
	var call func(context.Context, string, ...gen.RequestEditorFn) (*http.Response, error)
	switch action {
	case Resolve:
		call = c.gen.IssuesResolveCreate
	case Reopen:
		call = c.gen.IssuesReopenCreate
	case Mute:
		call = c.gen.IssuesMuteCreate
	case Unmute:
		call = c.gen.IssuesUnmuteCreate
	default:
		return Issue{}, fmt.Errorf("unknown triage action %q", action)
	}
	resp, err := call(ctx, issueID)
	if err != nil {
		return Issue{}, err
	}
	var body gen.Issue
	if err := decodeJSON(resp, &body); err != nil {
		return Issue{}, err
	}
	return issueOf(body), nil
}
