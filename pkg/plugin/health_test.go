// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type fakeProber struct {
	projects    []bugsink.Project
	projectsErr error
	issueErr    error
	eventsErr   error
	triageErr   error
	commentErr  error
	commentCode int
	ran         []string
	triaged     []string
	commented   []string
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

func (f *fakeProber) Triage(_ context.Context, id string, action bugsink.TriageAction, _ bugsink.MuteTerms) (bugsink.Issue, error) {
	f.ran = append(f.ran, "triage")
	f.triaged = append(f.triaged, id+" "+string(action))
	return bugsink.Issue{}, f.triageErr
}

func (f *fakeProber) AddComment(_ context.Context, issueID, text string) (int, error) {
	f.ran = append(f.ran, "comment")
	f.commented = append(f.commented, fmt.Sprintf("%s %q", issueID, text))
	return f.commentCode, f.commentErr
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
			res := checkHealth(ctx, testURL, defaultHealthTimeout, tt.f, false, false)
			if res.Status != tt.status || res.Message != tt.message {
				t.Fatalf("got %v %q\nwant %v %q", res.Status, res.Message, tt.status, tt.message)
			}
			if tt.ran != nil && fmt.Sprint(tt.f.ran) != fmt.Sprint(tt.ran) {
				t.Fatalf("probes run = %v, want %v", tt.f.ran, tt.ran)
			}
		})
	}
}

func TestCheckHealthTriageOffNeverTriages(t *testing.T) {
	for _, f := range []*fakeProber{
		{projects: named("a"), issueErr: apiErr(404, ""), eventsErr: apiErr(400, "")},
		{projects: named("a"), issueErr: apiErr(404, ""), eventsErr: apiErr(403, bugsink.EventsRead)},
		{projectsErr: apiErr(401, "")},
	} {
		checkHealth(context.Background(), testURL, defaultHealthTimeout, f, false, true)
		if len(f.triaged) != 0 {
			t.Fatalf("Triage called with %v while triage is off", f.triaged)
		}
	}
}

func TestCheckHealthTriage(t *testing.T) {
	has404, has400 := apiErr(404, ""), apiErr(400, "")
	connected := "Connected to Bugsink. Projects visible to this token: a."
	tests := []struct {
		name    string
		f       *fakeProber
		status  backend.HealthStatus
		message string
		triaged []string
	}{
		{"token can triage", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400, triageErr: has404},
			backend.HealthStatusOk, connected + " The token can triage issues.", []string{absentIssueID + " resolve"}},
		{"token lacks issues:triage", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400, triageErr: apiErr(403, bugsink.IssuesTriage)},
			backend.HealthStatusError, "The API token lacks: issues:triage.", []string{absentIssueID + " resolve"}},
		{"token lacks events:read and issues:triage", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: apiErr(403, bugsink.EventsRead), triageErr: apiErr(403, bugsink.IssuesTriage)},
			backend.HealthStatusError, "The API token lacks: events:read, issues:triage.", []string{absentIssueID + " resolve"}},
		{"200 cannot tell", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400},
			backend.HealthStatusError, "Bugsink answered HTTP 200 to a check for issues:triage.", []string{absentIssueID + " resolve"}},
		{"200 without Bugsink JSON cannot tell", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400, triageErr: fmt.Errorf("%w: %w", bugsink.ErrNotBugsink, apiErr(200, ""))},
			backend.HealthStatusError, "Bugsink answered HTTP 200 to a check for issues:triage.", []string{absentIssueID + " resolve"}},
		{"405 cannot tell", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400, triageErr: apiErr(405, "")},
			backend.HealthStatusError, "Bugsink answered HTTP 405 to a check for issues:triage.", []string{absentIssueID + " resolve"}},
		{"403 without a capability cannot tell", &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400, triageErr: apiErr(403, "")},
			backend.HealthStatusError, "Bugsink answered HTTP 403 to a check for issues:triage.", []string{absentIssueID + " resolve"}},
		{"401 on the first probe", &fakeProber{projectsErr: apiErr(401, "")},
			backend.HealthStatusError, "Bugsink rejected the API token. Check that it was copied in full and has not been deleted.", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultHealthTimeout)
			defer cancel()
			res := checkHealth(ctx, testURL, defaultHealthTimeout, tt.f, true, false)
			if res.Status != tt.status || res.Message != tt.message {
				t.Fatalf("got %v %q\nwant %v %q", res.Status, res.Message, tt.status, tt.message)
			}
			if tt.triaged != nil && fmt.Sprint(tt.f.triaged) != fmt.Sprint(tt.triaged) {
				t.Fatalf("triaged = %v, want %v", tt.f.triaged, tt.triaged)
			}
			if len(tt.f.triaged) > 1 {
				t.Fatalf("Triage called %d times", len(tt.f.triaged))
			}
		})
	}
}

func TestCheckHealthCommentProbeNeedsBothSwitches(t *testing.T) {
	for _, sw := range []struct{ triage, attribution bool }{{false, false}, {false, true}, {true, false}} {
		f := &fakeProber{projects: named("a"), issueErr: apiErr(404, ""), eventsErr: apiErr(400, ""), triageErr: apiErr(404, ""), commentErr: apiErr(400, "")}
		res := checkHealth(context.Background(), testURL, defaultHealthTimeout, f, sw.triage, sw.attribution)
		if len(f.commented) != 0 {
			t.Fatalf("triage %v attribution %v: AddComment called with %v", sw.triage, sw.attribution, f.commented)
		}
		if res.Status != backend.HealthStatusOk || strings.Contains(res.Message, "attributed") {
			t.Fatalf("triage %v attribution %v: got %v %q", sw.triage, sw.attribution, res.Status, res.Message)
		}
	}
}

func TestCheckHealthComment(t *testing.T) {
	has404, has400 := apiErr(404, ""), apiErr(400, "")
	canTriage := "Connected to Bugsink. Projects visible to this token: a. The token can triage issues."
	mayNot := func(status int) string {
		return fmt.Sprintf("%s Bugsink answered HTTP %d to a check for issues:comment, so actions may not be attributed.", canTriage, status)
	}
	probed := []string{absentIssueID + ` ""`}
	tests := []struct {
		name        string
		triageErr   error
		commentErr  error
		commentCode int
		status      backend.HealthStatus
		message     string
		commented   []string
	}{
		{"token can comment", has404, has400, 0,
			backend.HealthStatusOk, canTriage + " Actions are attributed by a comment.", probed},
		{"token lacks issues:comment", has404, apiErr(403, bugsink.IssuesComment), 0,
			backend.HealthStatusOk, canTriage + " It lacks issues:comment, so actions are not attributed.", probed},
		{"token lacks issues:triage and issues:comment", apiErr(403, bugsink.IssuesTriage), apiErr(403, bugsink.IssuesComment), 0,
			backend.HealthStatusError, "The API token lacks: issues:triage.", probed},
		{"token lacks issues:triage, can comment", apiErr(403, bugsink.IssuesTriage), has400, 0,
			backend.HealthStatusError, "The API token lacks: issues:triage.", probed},
		{"200 accepted the probe", has404, nil, 200,
			backend.HealthStatusError, "Bugsink answered HTTP 200 to a check for issues:comment.", probed},
		{"201 accepted the probe", has404, nil, 201,
			backend.HealthStatusError, "Bugsink answered HTTP 201 to a check for issues:comment.", probed},
		{"404 cannot tell", has404, has404, 0, backend.HealthStatusOk, mayNot(404), probed},
		{"405 cannot tell", has404, apiErr(405, ""), 0, backend.HealthStatusOk, mayNot(405), probed},
		{"500 cannot tell", has404, apiErr(500, ""), 0, backend.HealthStatusOk, mayNot(500), probed},
		{"403 without a capability cannot tell", has404, apiErr(403, ""), 0, backend.HealthStatusOk, mayNot(403), probed},
		{"403 naming another capability cannot tell", has404, apiErr(403, bugsink.IssuesTriage), 0, backend.HealthStatusOk, mayNot(403), probed},
		{"401 on the comment probe", has404, apiErr(401, ""), 0,
			backend.HealthStatusError, "Bugsink rejected the API token. Check that it was copied in full and has not been deleted.", probed},
		{"unreachable on the comment probe", has404, fmt.Errorf("Post %q: %w", "http://x/", errors.New("connection reset by peer")), 0,
			backend.HealthStatusError, "Cannot reach Bugsink at http://bugsink.example:8000/: connection reset by peer.", probed},
		{"timeout on the comment probe", has404, fmt.Errorf("Post %q: %w", "http://x/", context.DeadlineExceeded), 0,
			backend.HealthStatusError, "Bugsink at http://bugsink.example:8000/ did not answer within 10 seconds.", probed},
		{"triage probe fails first", apiErr(500, ""), has400, 0,
			backend.HealthStatusError, "Bugsink answered HTTP 500 to a check for issues:triage.", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultHealthTimeout)
			defer cancel()
			f := &fakeProber{projects: named("a"), issueErr: has404, eventsErr: has400, triageErr: tt.triageErr, commentErr: tt.commentErr, commentCode: tt.commentCode}
			res := checkHealth(ctx, testURL, defaultHealthTimeout, f, true, true)
			if res.Status != tt.status || res.Message != tt.message {
				t.Fatalf("got %v %q\nwant %v %q", res.Status, res.Message, tt.status, tt.message)
			}
			if fmt.Sprint(f.commented) != fmt.Sprint(tt.commented) {
				t.Fatalf("commented = %v, want %v", f.commented, tt.commented)
			}
			if len(f.commented) > 0 && fmt.Sprint(f.ran) != "[projects issue events triage comment]" {
				t.Fatalf("probes run = %v, want the comment probe after the triage probe", f.ran)
			}
		})
	}
}
