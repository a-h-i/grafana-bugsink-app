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

func TestNewNormalisesBaseURL(t *testing.T) {
	for _, suffix := range []string{"", "/", "/api/canonical/0", "/api/canonical/0/"} {
		t.Run("suffix "+suffix, func(t *testing.T) {
			var path string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path = r.URL.Path
				_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[]}`)
			}))
			defer srv.Close()
			c, err := New(srv.URL+suffix, "t", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.ListProjects(context.Background()); err != nil {
				t.Fatal(err)
			}
			if path != "/api/canonical/0/projects/" {
				t.Fatalf("path = %q", path)
			}
		})
	}

	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[]}`)
	}))
	defer srv.Close()
	c, err := New(srv.URL+"/bugsink/", "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListProjects(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/bugsink/api/canonical/0/projects/" {
		t.Fatalf("path = %q", path)
	}
}

func TestNewRejectsBadInput(t *testing.T) {
	for _, u := range []string{"", "ftp://h", "h:8000"} {
		if _, err := New(u, "t", nil); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("New(%q) error = %v, want ErrInvalidURL", u, err)
		}
	}
	if _, err := New("http://h", "  \n", nil); !errors.Is(err, ErrNoToken) {
		t.Errorf("error = %v, want ErrNoToken", err)
	}
}

func TestTokenIsTrimmedAndSentAsBearer(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[]}`)
	}))
	defer srv.Close()
	c, err := New(srv.URL, " abc\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListProjects(context.Background()); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer abc" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestListProjectsFollowsNext(t *testing.T) {
	var queries []string
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/canonical/0/projects/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		queries = append(queries, r.URL.Query().Get("cursor"))
		auths = append(auths, r.Header.Get("Authorization"))
		if r.URL.Query().Get("cursor") == "abc=" {
			_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[{"id":3,"name":"gamma"}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"next":"http://other.invalid/elsewhere/?cursor=abc%3D","previous":null,`+
			`"results":[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]}`)
	}))
	defer srv.Close()
	c, err := New(srv.URL, "abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Project{{1, "alpha"}, {2, "beta"}, {3, "gamma"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(queries) != 2 || queries[1] != "abc=" {
		t.Fatalf("cursors requested = %q", queries)
	}
	if auths[1] != "Bearer abc" {
		t.Fatalf("second page Authorization = %q", auths[1])
	}
}

func TestListProjectsStopsOnRepeatedCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"next":"http://x/p/?cursor=same","previous":null,"results":[]}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "t", nil)
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrNotBugsink) {
		t.Fatalf("error = %v, want ErrNotBugsink", err)
	}
}

func TestListProjectsNextWithoutCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"next":"http://x/p/?page=2","previous":null,"results":[]}`)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "t", nil)
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrNotBugsink) {
		t.Fatalf("error = %v, want ErrNotBugsink", err)
	}
}

func TestListProjectsOnHTMLIsNotBugsink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html></html>")
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "t", nil)
	_, err := c.ListProjects(context.Background())
	if !errors.Is(err, ErrNotBugsink) {
		t.Fatalf("error = %v, want ErrNotBugsink", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 200 {
		t.Fatalf("want *APIError with status 200, got %v", err)
	}
}

func answer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
}

// redirectTo is a server that answers every request with status and a Location on target, counting the requests.
func redirectTo(t *testing.T, status int, target string, count *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		http.Redirect(w, r, target+r.URL.Path, status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNeverFollowsRedirects(t *testing.T) {
	clients := map[string]*http.Client{"default client": nil, "given client": {}}
	calls := map[string]func(c *Client) error{
		"GET": func(c *Client) error { _, err := c.ListProjects(context.Background()); return err },
		"triage POST": func(c *Client) error {
			_, err := c.Triage(context.Background(), "SAMPLE-PYTHON-7", Resolve, MuteTerms{})
			return err
		},
		"mute POST": func(c *Client) error {
			_, err := c.Triage(context.Background(), "SAMPLE-PYTHON-7", MuteFor, MuteTerms{PeriodDay, 1, 0})
			return err
		},
		"comment POST": func(c *Client) error {
			_, err := c.AddComment(context.Background(), "SAMPLE-PYTHON-7", "Muted from Grafana by a.")
			return err
		},
	}
	for clientName, hc := range clients {
		for callName, call := range calls {
			for _, status := range []int{301, 302, 303, 307, 308} {
				t.Run(fmt.Sprintf("%s/%s/%d", clientName, callName, status), func(t *testing.T) {
					var targetSeen, redirectSeen atomic.Int32
					target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						targetSeen.Add(1)
						_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[]}`)
					}))
					t.Cleanup(target.Close)
					srv := redirectTo(t, status, target.URL, &redirectSeen)
					c, err := New(srv.URL, "secret-token", hc)
					if err != nil {
						t.Fatal(err)
					}
					err = call(c)
					var apiErr *APIError
					if !errors.As(err, &apiErr) || apiErr.Status != status {
						t.Fatalf("err %v, want an APIError with status %d", err, status)
					}
					if strings.Contains(err.Error(), target.URL) || strings.Contains(err.Error(), "secret-token") {
						t.Fatalf("error carries the Location or the token: %v", err)
					}
					if n := redirectSeen.Load(); n != 1 {
						t.Fatalf("Bugsink saw %d requests, want 1", n)
					}
					if n := targetSeen.Load(); n != 0 {
						t.Fatalf("the Location server saw %d requests", n)
					}
				})
			}
		}
	}
}

func TestCapabilityRefusal(t *testing.T) {
	cases := []struct {
		body string
		want Capability
	}{
		{`{"detail":"This token does not have the required capability: issues:read."}`, IssuesRead},
		{`{"detail":"This token is not allowed to access this project."}`, ""},
	}
	for _, tc := range cases {
		srv := answer(403, tc.body)
		c, _ := New(srv.URL, "t", nil)
		_, err := c.ListProjects(context.Background())
		srv.Close()
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("error = %v, want *APIError", err)
		}
		if apiErr.Status != 403 || apiErr.Capability != tc.want {
			t.Errorf("got status %d capability %q, want 403 %q", apiErr.Status, apiErr.Capability, tc.want)
		}
	}
}

func TestUnauthorized(t *testing.T) {
	srv := answer(401, `{"detail":"Invalid Bearer token."}`)
	defer srv.Close()
	c, _ := New(srv.URL, "secret-token", nil)
	_, err := c.ListProjects(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.Status != 401 || apiErr.Detail != "Invalid Bearer token." {
		t.Errorf("got %+v", apiErr)
	}
	if got := err.Error(); strings.Contains(got, "secret-token") {
		t.Errorf("error leaks the token: %q", got)
	}
}

func TestProbeAnswers(t *testing.T) {
	srv := answer(404, `{"detail":"No Issue matches the given query."}`)
	c, _ := New(srv.URL, "t", nil)
	_, err := c.GetIssue(context.Background(), "00000000-0000-0000-0000-000000000000")
	srv.Close()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Capability != "" {
		t.Errorf("GetIssue error = %v", err)
	}

	srv = answer(400, `{"issue":["This field is required."]}`)
	defer srv.Close()
	c, _ = New(srv.URL, "t", nil)
	err = c.ListEventsWithoutIssue(context.Background())
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Capability != "" {
		t.Errorf("ListEventsWithoutIssue error = %v", err)
	}
}

func TestAPIErrorFieldErrors(t *testing.T) {
	cases := []struct{ body, detail string }{
		{`{"sort":["Must be 'digest_order', 'last_seen', or 'digested_event_count'."]}`,
			"sort: Must be 'digest_order', 'last_seen', or 'digested_event_count'."},
		{`{"state":["bad state","second"],"order":["bad order"]}`, "order: bad order state: bad state"},
		{`{"detail":"nope"}`, "nope"},
	}
	for _, tc := range cases {
		e := newAPIError(400, []byte(tc.body))
		if e.Status != 400 || e.Detail != tc.detail {
			t.Errorf("body %s: got %+v, want detail %q", tc.body, e, tc.detail)
		}
	}
}

func TestAPIErrorListBody(t *testing.T) {
	cases := []struct{ body, detail string }{
		{`["Invalid issue identifier."]`, "Invalid issue identifier."},
		{`["first","second"]`, "first second"},
		{`[1]`, ""},
	}
	for _, tc := range cases {
		e := newAPIError(400, []byte(tc.body))
		if e.Status != 400 || e.Detail != tc.detail {
			t.Errorf("body %s: got %+v, want detail %q", tc.body, e, tc.detail)
		}
	}
}

func TestDecodeTooLarge(t *testing.T) {
	respOf := func(body string) *http.Response {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
	}
	padded := func(n int) string { return `{"a":1}` + strings.Repeat(" ", n-len(`{"a":1}`)) }

	var v map[string]int
	if err := decodeJSON(respOf(padded(maxBodyBytes)), &v); err != nil || v["a"] != 1 {
		t.Fatalf("exact limit: %v %v", err, v)
	}
	if err := decodeJSON(respOf(padded(maxBodyBytes+1)), &v); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("error = %v, want ErrTooLarge", err)
	}
}
