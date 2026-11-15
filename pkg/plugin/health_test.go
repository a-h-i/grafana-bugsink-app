// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type fakeProber struct {
	projects    []bugsink.Project
	projectsErr error
	issueErr    error
	eventsErr   error
	ran         []string
}

func (f *fakeProber) ListProjects(context.Context) ([]bugsink.Project, error) {
	f.ran = append(f.ran, "projects")
	return f.projects, f.projectsErr
}

func (f *fakeProber) GetIssue(_ context.Context, id string) (bugsink.Issue, error) {
	f.ran = append(f.ran, "issue")
	return bugsink.Issue{}, f.issueErr
}

func (f *fakeProber) ListEventsWithoutIssue(context.Context) error {
	f.ran = append(f.ran, "events")
	return f.eventsErr
}

func apiErr(status int, c bugsink.Capability) error {
	return &bugsink.APIError{Status: status, Capability: c}
}

func named(names ...string) []bugsink.Project {
	var ps []bugsink.Project
	for i, n := range names {
		ps = append(ps, bugsink.Project{ID: i + 1, Name: n})
	}
	return ps
}

func numbered(n int) []bugsink.Project {
	var names []string
	for i := 1; i <= n; i++ {
		names = append(names, fmt.Sprintf("p%d", i))
	}
	return named(names...)
}

const (
	testURL       = "http://bugsink.example:8000/"
	notBugsinkFmt = "http://bugsink.example:8000/ did not answer like a Bugsink API (HTTP %s). Check the URL: it is the address of Bugsink itself, without /api."
)

func TestCheckHealthMessages(t *testing.T) {
	has404, has400 := apiErr(404, ""), apiErr(400, "")
	tests := []struct {
		name    string
		f       *fakeProber
		status  backend.HealthStatus
		message string
		ran     []string
	}{
		{"all present, two projects", &fakeProber{projects: named("sample-python", "sample-js"), issueErr: has404, eventsErr: has400},
			backend.HealthStatusOk, "Connected to Bugsink. Projects visible to this token: sample-python, sample-js.", []string{"projects", "issue", "events"}},
		{"all present, no projects", &fakeProber{issueErr: has404, eventsErr: has400},
			backend.HealthStatusOk, "Connected to Bugsink. This token sees no projects.", nil},
		{"all present, ten projects", &fakeProber{projects: numbered(10), issueErr: has404, eventsErr: has400},
			backend.HealthStatusOk, "Connected to Bugsink. Projects visible to this token: p1, p2, p3, p4, p5, p6, p7, p8, p9, p10.", nil},
		{"all present, twelve projects", &fakeProber{projects: numbered(12), issueErr: has404, eventsErr: has400},
			backend.HealthStatusOk, "Connected to Bugsink. Projects visible to this token: p1, p2, p3, p4, p5, p6, p7, p8, p9, p10 and 2 more.", nil},
		{"lacks issues:read and events:read", &fakeProber{projects: named("a"), issueErr: apiErr(403, bugsink.IssuesRead), eventsErr: apiErr(403, bugsink.EventsRead)},
			backend.HealthStatusError, "The API token lacks: issues:read, events:read.", nil},
		{"lacks projects:read only", &fakeProber{projectsErr: apiErr(403, bugsink.ProjectsRead), issueErr: has404, eventsErr: has400},
			backend.HealthStatusError, "The API token lacks: projects:read.", []string{"projects", "issue", "events"}},
		{"lacks all three", &fakeProber{projectsErr: apiErr(403, bugsink.ProjectsRead), issueErr: apiErr(403, bugsink.IssuesRead), eventsErr: apiErr(403, bugsink.EventsRead)},
			backend.HealthStatusError, "The API token lacks: projects:read, issues:read, events:read.", nil},
		{"401 on the first probe", &fakeProber{projectsErr: apiErr(401, "")},
			backend.HealthStatusError, "Bugsink rejected the API token. Check that it was copied in full and has not been deleted.", []string{"projects"}},
		{"401 on a later probe", &fakeProber{projects: named("a"), issueErr: apiErr(401, "")},
			backend.HealthStatusError, "Bugsink rejected the API token. Check that it was copied in full and has not been deleted.", []string{"projects", "issue"}},
		{"first probe 404 JSON", &fakeProber{projectsErr: apiErr(404, "")},
			backend.HealthStatusError, fmt.Sprintf(notBugsinkFmt, "404"), []string{"projects"}},
		{"first probe 500", &fakeProber{projectsErr: apiErr(500, "")},
			backend.HealthStatusError, fmt.Sprintf(notBugsinkFmt, "500"), nil},
		{"first probe 403 without capability", &fakeProber{projectsErr: apiErr(403, "")},
			backend.HealthStatusError, fmt.Sprintf(notBugsinkFmt, "403"), nil},
		{"first probe ErrNotBugsink on 200", &fakeProber{projectsErr: fmt.Errorf("%w: %w", bugsink.ErrNotBugsink, apiErr(200, ""))},
			backend.HealthStatusError, fmt.Sprintf(notBugsinkFmt, "200"), []string{"projects"}},
		{"first probe pagination anomaly", &fakeProber{projectsErr: fmt.Errorf("%w: next page has no cursor", bugsink.ErrNotBugsink)},
			backend.HealthStatusError, fmt.Sprintf(notBugsinkFmt, "200"), nil},
		{"second probe 500", &fakeProber{projects: named("a"), issueErr: apiErr(500, ""), eventsErr: has400},
			backend.HealthStatusError, "Bugsink answered HTTP 500 to a check for issues:read.", nil},
		{"third probe 502", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: apiErr(502, "")},
			backend.HealthStatusError, "Bugsink answered HTTP 502 to a check for events:read.", nil},
		{"unexpected answer outranks lacks", &fakeProber{projectsErr: apiErr(403, bugsink.ProjectsRead), issueErr: apiErr(500, ""), eventsErr: has400},
			backend.HealthStatusError, "Bugsink answered HTTP 500 to a check for issues:read.", nil},
		{"connection refused", &fakeProber{projectsErr: fmt.Errorf("Get %q: %w", "http://x/?q=1", errors.New("connection refused"))},
			backend.HealthStatusError, "Cannot reach Bugsink at http://bugsink.example:8000/: connection refused.", []string{"projects"}},
		{"deadline exceeded", &fakeProber{projectsErr: fmt.Errorf("Get: %w", context.DeadlineExceeded)},
			backend.HealthStatusError, "Bugsink at http://bugsink.example:8000/ did not answer within 10 seconds.", []string{"projects"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultHealthTimeout)
			defer cancel()
			res := checkHealth(ctx, testURL, defaultHealthTimeout, tt.f)
			if res.Status != tt.status || res.Message != tt.message {
				t.Fatalf("got %v %q\nwant %v %q", res.Status, res.Message, tt.status, tt.message)
			}
			if tt.ran != nil && fmt.Sprint(tt.f.ran) != fmt.Sprint(tt.ran) {
				t.Fatalf("probes run = %v, want %v", tt.f.ran, tt.ran)
			}
		})
	}
}
