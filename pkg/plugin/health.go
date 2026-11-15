// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const defaultHealthTimeout = 10 * time.Second

const absentIssueID = "00000000-0000-0000-0000-000000000000"

const maxListedProjects = 10

const msgTokenRejected = "Bugsink rejected the API token. Check that it was copied in full and has not been deleted."

// prober is the part of the Bugsink client the health check needs.
type prober interface {
	ListProjects(ctx context.Context) ([]bugsink.Project, error)
	GetIssue(ctx context.Context, id string) (bugsink.Issue, error)
	ListEventsWithoutIssue(ctx context.Context) error
	Triage(ctx context.Context, id string, action bugsink.TriageAction, terms bugsink.MuteTerms) (bugsink.Issue, error)
	AddComment(ctx context.Context, issueID, text string) (int, error)
}

// probe is one capability check; a nil error or an APIError with status has means the token has it.
type probe struct {
	capability bugsink.Capability
	has        int
	run        func() error
}

func errorResult(format string, args ...any) *backend.CheckHealthResult {
	return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: fmt.Sprintf(format, args...)}
}

// checkHealth runs the capability probes, issues:triage when triage is on and issues:comment when attribution is also on, and renders the status and message.
func checkHealth(ctx context.Context, url string, timeout time.Duration, p prober, triage, attribution bool) *backend.CheckHealthResult {
	var projects []bugsink.Project
	probes := []probe{
		{capability: bugsink.ProjectsRead, run: func() (err error) { projects, err = p.ListProjects(ctx); return }},
		{capability: bugsink.IssuesRead, has: 404, run: func() (err error) { _, err = p.GetIssue(ctx, absentIssueID); return }},
		{capability: bugsink.EventsRead, has: 400, run: func() error { return p.ListEventsWithoutIssue(ctx) }},
	}
	if triage {
		probes = append(probes, probe{capability: bugsink.IssuesTriage, has: 404, run: func() error {
			if _, err := p.Triage(ctx, absentIssueID, bugsink.Resolve, bugsink.MuteTerms{}); err != nil {
				return err
			}
			return &bugsink.APIError{Status: http.StatusOK} // no issue has the absent id, so a success tells nothing
		}})
	}

	var lacking []string
	for i, pr := range probes {
		err := pr.run()
		if err == nil {
			continue
		}
		var apiErr *bugsink.APIError
		isAPI := errors.As(err, &apiErr)
		switch {
		case !isAPI && !errors.Is(err, bugsink.ErrNotBugsink):
			return unreachable(ctx, err, url, timeout)
		case isAPI && apiErr.Status == 401:
			return errorResult(msgTokenRejected)
		case isAPI && apiErr.Capability != "":
			lacking = append(lacking, string(pr.capability))
		case isAPI && pr.has != 0 && apiErr.Status == pr.has:
		case i == 0:
			status := 200
			if isAPI {
				status = apiErr.Status
			}
			return errorResult("%s did not answer like a Bugsink API (HTTP %d). Check the URL: it is the address of Bugsink itself, without /api.", url, status)
		case isAPI:
			return errorResult("Bugsink answered HTTP %d to a check for %s.", apiErr.Status, pr.capability)
		default:
			return errorResult("Bugsink answered HTTP 200 to a check for %s.", pr.capability)
		}
	}

	commentLine := ""
	if triage && attribution {
		var failed *backend.CheckHealthResult
		if commentLine, failed = commentProbe(ctx, url, timeout, p); failed != nil {
			return failed
		}
	}
	if len(lacking) > 0 {
		return errorResult("The API token lacks: %s.", strings.Join(lacking, ", "))
	}
	message := connectedMessage(projects)
	if triage {
		message += " The token can triage issues."
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: message + commentLine}
}

// commentProbe posts a blank comment on the absent issue and returns the sentence on attribution, or the error result.
func commentProbe(ctx context.Context, url string, timeout time.Duration, p prober) (string, *backend.CheckHealthResult) {
	status, err := p.AddComment(ctx, absentIssueID, "")
	var apiErr *bugsink.APIError
	switch {
	case err == nil:
		return "", errorResult("Bugsink answered HTTP %d to a check for %s.", status, bugsink.IssuesComment)
	case !errors.As(err, &apiErr):
		return "", unreachable(ctx, err, url, timeout)
	case apiErr.Status == 401:
		return "", errorResult(msgTokenRejected)
	case apiErr.Status == 400:
		return " Actions are attributed by a comment.", nil
	case apiErr.Status == 403 && apiErr.Capability == bugsink.IssuesComment:
		return " It lacks issues:comment, so actions are not attributed.", nil
	default:
		return fmt.Sprintf(" Bugsink answered HTTP %d to a check for %s, so actions may not be attributed.", apiErr.Status, bugsink.IssuesComment), nil
	}
}

// unreachable is the error result for a probe that got no answer from Bugsink.
func unreachable(ctx context.Context, err error, url string, timeout time.Duration) *backend.CheckHealthResult {
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return errorResult("Bugsink at %s did not answer within %s seconds.", url, formatSeconds(timeout))
	}
	return errorResult("Cannot reach Bugsink at %s: %s.", url, rootCause(err))
}

func connectedMessage(projects []bugsink.Project) string {
	if len(projects) == 0 {
		return "Connected to Bugsink. This token sees no projects."
	}
	shown := projects[:min(len(projects), maxListedProjects)]
	names := make([]string, len(shown))
	for i, pr := range shown {
		names[i] = pr.Name
	}
	list := strings.Join(names, ", ")
	if extra := len(projects) - len(shown); extra > 0 {
		list += fmt.Sprintf(" and %d more", extra)
	}
	return "Connected to Bugsink. Projects visible to this token: " + list + "."
}

// rootCause is the innermost error, which carries no request URL or header.
func rootCause(err error) string {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return err.Error()
		}
		err = inner
	}
}

func formatSeconds(d time.Duration) string {
	return strconv.FormatFloat(math.Round(d.Seconds()*10)/10, 'f', -1, 64)
}
