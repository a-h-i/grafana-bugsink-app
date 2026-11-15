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
	Resolve       TriageAction = "resolve"
	ResolveLatest TriageAction = "resolve-latest"
	ResolveNext   TriageAction = "resolve-next"
	Reopen        TriageAction = "reopen"
	Mute          TriageAction = "mute"
	MuteFor       TriageAction = "mute-for"
	MuteUntil     TriageAction = "mute-until"
	Unmute        TriageAction = "unmute"
)

// Period is the unit of a timed mute.
type Period string

const (
	PeriodMinute Period = "minute"
	PeriodHour   Period = "hour"
	PeriodDay    Period = "day"
	PeriodWeek   Period = "week"
	PeriodMonth  Period = "month"
	PeriodYear   Period = "year"
)

// MaxPeriods and MaxThreshold bound MuteTerms.Periods and MuteTerms.Threshold.
const (
	MaxPeriods   = 100
	MaxThreshold = 1000000
)

// MuteTerms are the terms of MuteFor and MuteUntil; Threshold is sent by MuteUntil only.
type MuteTerms struct {
	Period             Period
	Periods, Threshold int
}

// Valid reports whether p is one of the six periods.
func (p Period) Valid() bool {
	switch p {
	case PeriodMinute, PeriodHour, PeriodDay, PeriodWeek, PeriodMonth, PeriodYear:
		return true
	}
	return false
}

// check refuses terms Bugsink would refuse or could not store, before any request.
func (t MuteTerms) check(withThreshold bool) error {
	switch {
	case !t.Period.Valid():
		return fmt.Errorf("unknown mute period %q", t.Period)
	case t.Periods < 1 || t.Periods > MaxPeriods:
		return fmt.Errorf("number of periods %d is outside 1 to %d", t.Periods, MaxPeriods)
	case withThreshold && (t.Threshold < 1 || t.Threshold > MaxThreshold):
		return fmt.Errorf("threshold %d is outside 1 to %d", t.Threshold, MaxThreshold)
	}
	return nil
}

// Triage runs action on the issue; MuteFor and MuteUntil send terms, the others send no body.
func (c *Client) Triage(ctx context.Context, issueID string, action TriageAction, terms MuteTerms) (Issue, error) {
	resp, err := c.triageRequest(ctx, issueID, action, terms)
	if err != nil {
		return Issue{}, err
	}
	var body gen.Issue
	if err := decodeJSON(resp, &body); err != nil {
		return Issue{}, err
	}
	return issueOf(body), nil
}

func (c *Client) triageRequest(ctx context.Context, id string, action TriageAction, terms MuteTerms) (*http.Response, error) {
	switch action {
	case Resolve:
		return c.gen.IssuesResolveCreate(ctx, id)
	case ResolveLatest:
		return c.gen.IssuesResolveLatestCreate(ctx, id)
	case ResolveNext:
		return c.gen.IssuesResolveNextCreate(ctx, id)
	case Reopen:
		return c.gen.IssuesReopenCreate(ctx, id)
	case Mute:
		return c.gen.IssuesMuteCreate(ctx, id)
	case Unmute:
		return c.gen.IssuesUnmuteCreate(ctx, id)
	case MuteFor:
		if err := terms.check(false); err != nil {
			return nil, err
		}
		return c.gen.IssuesMuteForCreate(ctx, id, gen.IssueMuteFor{
			PeriodName: gen.PeriodNameEnum(terms.Period), NrOfPeriods: terms.Periods})
	case MuteUntil:
		if err := terms.check(true); err != nil {
			return nil, err
		}
		return c.gen.IssuesMuteUntilCreate(ctx, id, gen.IssueMuteUntil{
			PeriodName: gen.PeriodNameEnum(terms.Period), NrOfPeriods: terms.Periods, GteThreshold: terms.Threshold})
	default:
		return nil, fmt.Errorf("unknown triage action %q", action)
	}
}

// AddComment posts text unchanged as a comment on the issue and returns Bugsink's status; Bugsink refuses a blank one.
func (c *Client) AddComment(ctx context.Context, issueID, text string) (int, error) {
	resp, err := c.gen.IssueCommentsCreate(ctx, gen.IssueComment{Issue: issueID, Comment: text})
	if err := expectSuccess(resp, err); err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}
