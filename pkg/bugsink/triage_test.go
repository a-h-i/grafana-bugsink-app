// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const triageUUID = "0b1e1c9e-1a2b-4c3d-8e4f-0123456789ab"

func TestTriageSendsBodilessPost(t *testing.T) {
	resolved := strings.Replace(issueJSON("SAMPLE-1", "2026-01-02T00:00:00Z"), `"is_resolved":false`, `"is_resolved":true`, 1)
	for _, action := range []TriageAction{Resolve, Reopen, Mute, Unmute} {
		t.Run(string(action), func(t *testing.T) {
			var method, path, query, auth string
			var body []byte
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				method, path, query, auth = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
				body, _ = io.ReadAll(r.Body)
				_, _ = w.Write([]byte(resolved))
			})
			issue, err := c.Triage(context.Background(), triageUUID, action)
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodPost || path != "/api/canonical/0/issues/"+triageUUID+"/"+string(action)+"/" || query != "" || len(body) != 0 || auth != "Bearer tok" {
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
	if _, err := c.Triage(context.Background(), "SAMPLE-PYTHON-7", Mute); err != nil {
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
			_, err = client.Triage(context.Background(), triageUUID, Resolve)
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
	if _, err := c.Triage(context.Background(), triageUUID, TriageAction("delete")); err == nil {
		t.Fatal("unknown action accepted")
	}
	if requests.Load() != 0 {
		t.Fatal("unknown action reached Bugsink")
	}
}
