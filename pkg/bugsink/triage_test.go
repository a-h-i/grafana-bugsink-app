// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	triageUUID = "0b1e1c9e-1a2b-4c3d-8e4f-0123456789ab"
	absentUUID = "00000000-0000-0000-0000-000000000000"
)

func TestTriageSendsBodilessPost(t *testing.T) {
	resolved := strings.Replace(issueJSON("SAMPLE-1", "2026-01-02T00:00:00Z"), `"is_resolved":false`, `"is_resolved":true`, 1)
	for _, action := range []TriageAction{Resolve, Reopen, Mute, Unmute, ResolveLatest, ResolveNext} {
		t.Run(string(action), func(t *testing.T) {
			var method, path, query, auth string
			var body []byte
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				method, path, query, auth = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
				body, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(resolved))
			})
			issue, err := c.Triage(context.Background(), triageUUID, action, MuteTerms{})
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodPost || path != "/api/canonical/0/issues/"+triageUUID+"/"+string(action)+"/" || query != "" || len(body) != 0 || auth != "Bearer "+testToken {
				t.Fatalf("%s %s ?%s body %q auth %q", method, path, query, body, auth)
			}
			if !issue.IsResolved || issue.FriendlyID != "SAMPLE-1" || issue.ID != triageUUID {
				t.Fatalf("issue %+v", issue)
			}
		})
	}
}

func TestTriageShortID(t *testing.T) {
	var path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(issueJSON("SAMPLE-PYTHON-7", "2026-01-02T00:00:00Z")))
	})
	if _, err := c.Triage(context.Background(), "SAMPLE-PYTHON-7", Mute, MuteTerms{}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/mute/" {
		t.Fatalf("path %q", path)
	}
}

func TestTriageRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		body   string
		want   APIError
	}{
		"state":      {400, `{"detail":"Issue is already resolved."}`, APIError{Status: 400, Detail: "Issue is already resolved."}},
		"capability": {403, `{"detail":"This token does not have the required capability: issues:triage."}`, APIError{Status: 403, Detail: "This token does not have the required capability: issues:triage.", Capability: IssuesTriage}},
		"missing":    {404, `{"detail":"No Issue matches the given query."}`, APIError{Status: 404, Detail: "No Issue matches the given query."}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := answer(c.status, c.body)
			defer srv.Close()
			client, err := New(srv.URL, "secret-token", nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Triage(context.Background(), triageUUID, Resolve, MuteTerms{})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || *apiErr != c.want {
				t.Fatalf("err %v, want %+v", err, c.want)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error carries the token: %v", err)
			}
		})
	}
}

func TestTriageUnknownAction(t *testing.T) {
	var requests atomic.Int32
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
	if _, err := c.Triage(context.Background(), triageUUID, TriageAction("delete"), MuteTerms{}); err == nil {
		t.Fatal("unknown action accepted")
	}
	if requests.Load() != 0 {
		t.Fatal("unknown action reached Bugsink")
	}
}

func TestTriageSendsMuteTerms(t *testing.T) {
	for name, c := range map[string]struct {
		action TriageAction
		terms  MuteTerms
		body   string
	}{
		"mute-for":   {MuteFor, MuteTerms{PeriodDay, 2, 9}, `{"nr_of_periods":2,"period_name":"day"}`},
		"mute-until": {MuteUntil, MuteTerms{PeriodHour, 1, 10}, `{"gte_threshold":10,"nr_of_periods":1,"period_name":"hour"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var method, path, query, contentType string
			var body []byte
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				method, path, query, contentType = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Content-Type")
				body, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(issueJSON("SAMPLE-1", "2026-01-02T00:00:00Z")))
			})
			issue, err := client.Triage(context.Background(), triageUUID, c.action, c.terms)
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodPost || path != "/api/canonical/0/issues/"+triageUUID+"/"+name+"/" || query != "" {
				t.Fatalf("%s %s ?%s", method, path, query)
			}
			if string(body) != c.body || contentType != "application/json" {
				t.Fatalf("body %q content type %q, want %q", body, contentType, c.body)
			}
			if issue.FriendlyID != "SAMPLE-1" {
				t.Fatalf("issue %+v", issue)
			}
		})
	}
}

func TestTriageUnknownPeriod(t *testing.T) {
	var requests atomic.Int32
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
	for _, p := range []Period{"fortnight", "Day", ""} {
		for _, action := range []TriageAction{MuteFor, MuteUntil} {
			if _, err := c.Triage(context.Background(), triageUUID, action, MuteTerms{p, 1, 1}); err == nil {
				t.Fatalf("%s with period %q accepted", action, p)
			}
		}
	}
	if requests.Load() != 0 {
		t.Fatal("an unknown period reached Bugsink")
	}
}

func TestTriageTermsOutOfRange(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer srv.Close()
	client, err := New(srv.URL, "secret-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		action TriageAction
		terms  MuteTerms
	}{
		"for no periods":        {MuteFor, MuteTerms{PeriodDay, 0, 1}},
		"for negative periods":  {MuteFor, MuteTerms{PeriodDay, -1, 1}},
		"for 101 periods":       {MuteFor, MuteTerms{PeriodDay, 101, 1}},
		"until no periods":      {MuteUntil, MuteTerms{PeriodDay, 0, 1}},
		"until 101 periods":     {MuteUntil, MuteTerms{PeriodDay, 101, 1}},
		"until no threshold":    {MuteUntil, MuteTerms{PeriodDay, 1, 0}},
		"until large threshold": {MuteUntil, MuteTerms{PeriodDay, 1, 1000001}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.Triage(context.Background(), triageUUID, c.action, c.terms)
			if err == nil {
				t.Fatal("terms accepted")
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error carries the token: %v", err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("out-of-range terms reached Bugsink")
	}
}

func TestTriageTermsInRange(t *testing.T) {
	var bodies []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		_, _ = w.Write([]byte(issueJSON("SAMPLE-1", "2026-01-02T00:00:00Z")))
	})
	for _, call := range []struct {
		action TriageAction
		terms  MuteTerms
	}{
		{MuteFor, MuteTerms{PeriodMinute, 100, 0}},
		{MuteUntil, MuteTerms{PeriodYear, 1, 1000000}},
		{MuteUntil, MuteTerms{PeriodWeek, 3, 1}},
		{MuteFor, MuteTerms{PeriodMonth, 1, 0}},
	} {
		if _, err := c.Triage(context.Background(), triageUUID, call.action, call.terms); err != nil {
			t.Fatalf("%s %+v: %v", call.action, call.terms, err)
		}
	}
	want := []string{
		`{"nr_of_periods":100,"period_name":"minute"}`,
		`{"gte_threshold":1000000,"nr_of_periods":1,"period_name":"year"}`,
		`{"gte_threshold":1,"nr_of_periods":3,"period_name":"week"}`,
		`{"nr_of_periods":1,"period_name":"month"}`,
	}
	if strings.Join(bodies, "\n") != strings.Join(want, "\n") {
		t.Fatalf("bodies %q", bodies)
	}
}

func TestAddComment(t *testing.T) {
	var method, path, query, contentType string
	var body []byte
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, query, contentType = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Content-Type")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"id":373,"issue":%q,"project":1,"timestamp":"2026-11-15T05:00:00Z","comment":"x","user":null}`, triageUUID)
	})
	status, err := c.AddComment(context.Background(), triageUUID, "Resolved from Grafana by a.")
	if err != nil || status != http.StatusCreated {
		t.Fatalf("status %d err %v, want 201", status, err)
	}
	want := `{"comment":"Resolved from Grafana by a.","issue":"` + triageUUID + `"}`
	if method != http.MethodPost || path != "/api/canonical/0/issue-comments/" || query != "" || contentType != "application/json" {
		t.Fatalf("%s %s ?%s content type %q", method, path, query, contentType)
	}
	if string(body) != want {
		t.Fatalf("body %s, want %s", body, want)
	}
}

func TestAddCommentReturnsBugsinksStatus(t *testing.T) {
	srv := answer(http.StatusOK, `{}`)
	defer srv.Close()
	client, err := New(srv.URL, "secret-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status, err := client.AddComment(context.Background(), absentUUID, ""); err != nil || status != http.StatusOK {
		t.Fatalf("status %d err %v, want 200", status, err)
	}
}

func TestAddCommentRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		body   string
		want   APIError
	}{
		"capability": {403, `{"detail":"This token does not have the required capability: issues:comment."}`, APIError{Status: 403, Detail: "This token does not have the required capability: issues:comment.", Capability: IssuesComment}},
		"fields":     {400, `{"issue":["Issue not found."],"comment":["This field may not be blank."]}`, APIError{Status: 400, Detail: "comment: This field may not be blank. issue: Issue not found."}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := answer(c.status, c.body)
			defer srv.Close()
			client, err := New(srv.URL, "secret-token", nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.AddComment(context.Background(), absentUUID, "")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || *apiErr != c.want {
				t.Fatalf("err %v, want %+v", err, c.want)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error carries the token: %v", err)
			}
		})
	}
}
