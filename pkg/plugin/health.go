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

// prober is the part of the Bugsink client the health check needs.
type prober interface {
	ListProjects(ctx context.Context) ([]bugsink.Project, error)
	GetIssue(ctx context.Context, id string) (bugsink.Issue, error)
	ListEventsWithoutIssue(ctx context.Context) error
	Triage(ctx context.Context, id string, action bugsink.TriageAction) (bugsink.Issue, error)
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

// checkHealth runs the capability probes, issues:triage only when triage is on, and renders the status and message.
func checkHealth(ctx context.Context, url string, timeout time.Duration, p prober, triage bool) *backend.CheckHealthResult {
	var projects []bugsink.Project
	probes := []probe{
		{bugsink.ProjectsRead, 0, func() (err error) { projects, err = p.ListProjects(ctx); return }},
		{bugsink.IssuesRead, 404, func() (err error) { _, err = p.GetIssue(ctx, absentIssueID); return }},
		{bugsink.EventsRead, 400, func() error { return p.ListEventsWithoutIssue(ctx) }},
	}
	if triage {
		probes = append(probes, probe{bugsink.IssuesTriage, 404, func() error {
			if _, err := p.Triage(ctx, absentIssueID, bugsink.Resolve); err != nil {
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
			if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				return errorResult("Bugsink at %s did not answer within %s seconds.", url, formatSeconds(timeout))
			}
			return errorResult("Cannot reach Bugsink at %s: %s.", url, rootCause(err))
		case isAPI && apiErr.Status == 401:
			return errorResult("Bugsink rejected the API token. Check that it was copied in full and has not been deleted.")
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

	if len(lacking) > 0 {
		return errorResult("The API token lacks: %s.", strings.Join(lacking, ", "))
	}
	message := connectedMessage(projects)
	if triage {
		message += " The token can triage issues."
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: message}
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
