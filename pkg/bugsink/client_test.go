// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
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

func TestUnparsableLocationHidesToken(t *testing.T) {
	locations := map[string]func(auth string) string{
		"token": func(a string) string { return "%zz" + a },
		"long":  func(string) string { return "%zz" + strings.Repeat("x", 3000) },
	}
	for name, location := range locations {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", location(r.Header.Get("Authorization")))
				w.WriteHeader(http.StatusFound)
			}))
			t.Cleanup(srv.Close)
			for call, err := range refusals(t, srv, echoedToken) {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Status != http.StatusFound || holdsToken(err.Error(), echoedToken) {
					t.Errorf("%s: error %.300v, want an APIError with status 302", call, err)
				}
			}
		})
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
		e := newAPIError(400, []byte(tc.body), "")
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
		e := newAPIError(400, []byte(tc.body), "")
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

const echoedToken = "s3cret-Tok-value"

// holdsToken reports whether s contains the token in any letter case.
func holdsToken(s, token string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(token))
}

// echoing is a server that answers every request with status, contentType and the body made from its Authorization header.
func echoing(t *testing.T, status int, contentType string, body func(auth string) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body(r.Header.Get("Authorization")))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// refusals runs a read and a write call against srv with token and returns their errors by name.
func refusals(t *testing.T, srv *httptest.Server, token string) map[string]error {
	t.Helper()
	c, err := New(srv.URL, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := c.GetIssue(context.Background(), "X-1")
	_, writeErr := c.Triage(context.Background(), "X-1", Resolve, MuteTerms{})
	return map[string]error{"read": readErr, "write": writeErr}
}

func TestAPIErrorHidesToken(t *testing.T) {
	upper := strings.ToUpper(echoedToken)
	cases := []struct {
		name, contentType string
		status            int
		body              func(auth string) string
		detail            string
		capability        Capability
	}{
		{"detail with Bearer", "application/json", 400, func(a string) string { return `{"detail":"echo ` + a + `"}` },
			"echo Bearer [redacted]", ""},
		{"detail without Bearer", "application/json", 400, func(string) string { return `{"detail":"echo ` + echoedToken + `"}` },
			"echo [redacted]", ""},
		{"lower-case prefix", "application/json", 400, func(string) string { return `{"detail":"echo bearer ` + echoedToken + `"}` },
			"echo bearer [redacted]", ""},
		{"upper-case token", "application/json", 400, func(string) string { return `{"detail":"echo BEARER ` + upper + `"}` },
			"echo BEARER [redacted]", ""},
		{"twice", "application/json", 400, func(a string) string { return `{"detail":"` + a + ` and ` + echoedToken + `"}` },
			"Bearer [redacted] and [redacted]", ""},
		{"list body", "application/json", 400, func(string) string { return `["echo","` + echoedToken + `"]` },
			"echo [redacted]", ""},
		{"field error", "application/json", 400, func(string) string { return `{"q":["` + echoedToken + `"]}` },
			"q: [redacted]", ""},
		{"html body", "text/html", 400, func(a string) string { return "<p>" + a + "</p>" }, "", ""},
		{"capability", "application/json", 403,
			func(string) string { return `{"detail":"` + missingCapabilityPrefix + echoedToken + `."}` },
			missingCapabilityPrefix + "[redacted].", "[redacted]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := echoing(t, tc.status, tc.contentType, tc.body)
			for call, err := range refusals(t, srv, echoedToken) {
				var apiErr *APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("%s: error %v, want *APIError", call, err)
				}
				if apiErr.Status != tc.status || apiErr.Detail != tc.detail || apiErr.Capability != tc.capability {
					t.Errorf("%s: got %+v, want detail %q capability %q", call, apiErr, tc.detail, tc.capability)
				}
				if holdsToken(err.Error(), echoedToken) {
					t.Errorf("%s: error leaks the token: %q", call, err)
				}
			}
		})
	}
}

func TestAPIErrorTokenInsideWords(t *testing.T) {
	srv := echoing(t, 403, "application/json", func(string) string {
		return `{"detail":"This token is not allowed to access this project."}`
	})
	for call, err := range refusals(t, srv, "allow") {
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Detail != "This token is not [redacted]ed to access this project." {
			t.Errorf("%s: error %v", call, err)
		}
	}
}

func TestAPIErrorWithoutToken(t *testing.T) {
	type result struct {
		detail string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		e := newAPIError(400, []byte(`{"detail":"Invalid issue identifier."}`), "")
		resp := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`["no request"]`))}
		done <- result{e.Detail, readAPIError(resp)}
	}()
	var r result
	select {
	case r = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("an error without a token was not built within 2 seconds")
	}
	if r.detail != "Invalid issue identifier." {
		t.Fatalf("detail %q", r.detail)
	}
	var apiErr *APIError
	if !errors.As(r.err, &apiErr) || apiErr.Detail != "no request" {
		t.Fatalf("error %v", r.err)
	}
}

func TestForwardableWithShortTokenBuildsLittle(t *testing.T) {
	text := strings.Repeat("a", maxBodyBytes)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got := forwardable(text, "a")
	runtime.ReadMemStats(&after)
	if want := strings.Repeat(redacted, 50) + ellipsis; got != want {
		t.Fatalf("got %d characters: %.100q", utf8.RuneCountInString(got), got)
	}
	if built := after.TotalAlloc - before.TotalAlloc; built > 1<<20 {
		t.Fatalf("built %d bytes for a %d-byte text", built, len(text))
	}
}

func TestForwardableRepeatedLongToken(t *testing.T) {
	token := strings.Repeat("0123456789abcdef", 3)[:40]
	got := forwardable(strings.Repeat(token, 1000), token)
	if want := strings.Repeat(redacted, 50) + ellipsis; got != want {
		t.Fatalf("got %q", got)
	}
}

func TestAPIErrorIsCut(t *testing.T) {
	const ellipsis = "…"
	detailOfBody := func(t *testing.T, status int, body string) *APIError {
		t.Helper()
		srv := echoing(t, status, "application/json", func(string) string { return body })
		var apiErr *APIError
		if err := refusals(t, srv, echoedToken)["read"]; !errors.As(err, &apiErr) {
			t.Fatalf("error %v", err)
		}
		return apiErr
	}
	t.Run("2000 characters", func(t *testing.T) {
		long := strings.Repeat("é", 2000)
		got := detailOfBody(t, 400, `{"detail":"`+long+`"}`).Detail
		if want := strings.Repeat("é", 500) + ellipsis; got != want {
			t.Fatalf("detail has %d characters, valid UTF-8 %v: %q", utf8.RuneCountInString(got), utf8.ValidString(got), got)
		}
	})
	t.Run("500 characters", func(t *testing.T) {
		exact := strings.Repeat("x", 500)
		if got := detailOfBody(t, 400, `{"detail":"`+exact+`"}`).Detail; got != exact {
			t.Fatalf("detail %q", got)
		}
	})
	t.Run("token across the cut", func(t *testing.T) {
		got := detailOfBody(t, 400, `{"detail":"`+strings.Repeat("x", 495)+echoedToken+`"}`).Detail
		for i := 0; i+6 <= len(echoedToken); i++ {
			if strings.Contains(got, echoedToken[i:i+6]) {
				t.Fatalf("detail holds %q: %q", echoedToken[i:i+6], got)
			}
		}
		if want := strings.Repeat("x", 495) + "[reda" + ellipsis; got != want {
			t.Fatalf("detail %q", got)
		}
	})
	t.Run("capability", func(t *testing.T) {
		got := detailOfBody(t, 403, `{"detail":"`+missingCapabilityPrefix+`issues:read."}`)
		if got.Capability != IssuesRead {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestAPIErrorAtTheReadLimit(t *testing.T) {
	t.Run("detail within 10 MB", func(t *testing.T) {
		pad := maxBodyBytes - len(`{"detail":""}`) - len(echoedToken)
		srv := echoing(t, 400, "application/json", func(string) string {
			return `{"detail":"` + strings.Repeat("x", pad) + echoedToken + `"}`
		})
		for call, err := range refusals(t, srv, echoedToken) {
			var apiErr *APIError
			if !errors.As(err, &apiErr) || utf8.RuneCountInString(apiErr.Detail) != 501 || holdsToken(err.Error(), echoedToken) {
				t.Errorf("%s: error has %d bytes", call, len(err.Error()))
			}
		}
	})
	t.Run("error body over 10 MB", func(t *testing.T) {
		srv := echoing(t, 400, "application/json", func(a string) string {
			return `{"detail":"` + strings.Repeat("x", maxBodyBytes) + a + `"}`
		})
		for call, err := range refusals(t, srv, echoedToken) {
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Detail != "" || holdsToken(err.Error(), echoedToken) {
				t.Errorf("%s: error %.200v", call, err)
			}
		}
	})
	t.Run("answer over 10 MB", func(t *testing.T) {
		srv := echoing(t, 200, "application/json", func(a string) string {
			return `{"detail":"` + a + strings.Repeat("x", maxBodyBytes) + `"}`
		})
		c, _ := New(srv.URL, echoedToken, nil)
		if _, err := c.GetIssue(context.Background(), "X-1"); !errors.Is(err, ErrTooLarge) || holdsToken(err.Error(), echoedToken) {
			t.Fatalf("error %.200v", err)
		}
	})
}

// malformed is a server that writes raw text made from the request's Authorization header instead of an HTTP answer.
func malformed(t *testing.T, raw func(auth string) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString(raw(r.Header.Get("Authorization")))
		_ = buf.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTransportErrorHidesToken(t *testing.T) {
	cases := map[string]func(auth string) string{
		"header line":  func(a string) string { return "HTTP/1.1 200 OK\r\n" + a + "\r\n\r\n" },
		"status line":  func(a string) string { return a + "\r\n\r\n" },
		"status text":  func(a string) string { return "HTTP/1.1 " + strings.TrimPrefix(a, "Bearer ") + "\r\n\r\n" },
		"long header":  func(a string) string { return "HTTP/1.1 200 OK\r\n" + strings.Repeat("x", 495) + a + "\r\n\r\n" },
		"trailer":      func(a string) string { return chunkedOK + a + "\r\n\r\n" },
		"long trailer": func(string) string { return chunkedOK + strings.Repeat("x", 3000) + "\r\n\r\n" },
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			for call, err := range refusals(t, malformed(t, raw), echoedToken) {
				if err == nil || holdsToken(err.Error(), echoedToken) {
					t.Fatalf("%s: error %.300v", call, err)
				}
				if n := utf8.RuneCountInString(innermost(err).Error()); n > maxDetailChars+1 {
					t.Errorf("%s: innermost error has %d characters", call, n)
				}
			}
		})
	}
}

func TestRequestErrorsHoldNoQuery(t *testing.T) {
	const search = "searched-text"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/projects/") && !r.URL.Query().Has("cursor") {
			_, _ = fmt.Fprintf(w, `{"next":"http://x/p/?cursor=%s","previous":null,"results":[{"id":1,"name":"p"}]}`, echoedToken)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, echoedToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	calls := map[string]func() error{
		"cursor read": func() error { _, err := c.ListProjects(ctx); return err },
		"search read": func() error {
			_, err := c.ListIssues(ctx, IssueFilter{State: StateOpen, Query: search, Sort: SortLastSeen, Order: Desc}, time.Time{})
			return err
		},
		"write": func() error { _, err := c.Triage(ctx, "X-1", Resolve, MuteTerms{}); return err },
		"probe": func() error { return c.ListEventsWithoutIssue(ctx) },
	}
	for name, call := range calls {
		err := call()
		var urlErr *url.Error
		if !errors.As(err, &urlErr) {
			t.Fatalf("%s: error %v, want a url.Error", name, err)
		}
		if holdsToken(err.Error(), echoedToken) || strings.Contains(err.Error(), search) || strings.Contains(err.Error(), "?") {
			t.Errorf("%s: error holds the query: %v", name, err)
		}
		if !strings.Contains(urlErr.URL, srv.URL+apiPath+"/") {
			t.Errorf("%s: error URL %q, want the request's path", name, urlErr.URL)
		}
		if got := innermost(err).Error(); got != io.EOF.Error() {
			t.Errorf("%s: innermost error %q, want EOF", name, got)
		}
	}
}

func TestRequestErrorsHoldNoTokenInThePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	for _, tc := range []struct{ name, token, id string }{
		{"token", echoedToken, echoedToken},
		{"other case", echoedToken, "x-" + strings.ToUpper(echoedToken) + "-x"},
		{"escaped token", "s3cret Tok/value", "s3cret Tok/value"},
		{"long path", echoedToken, strings.Repeat("y", 600) + echoedToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(srv.URL, tc.token, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.GetIssue(context.Background(), tc.id)
			var urlErr *url.Error
			if !errors.As(err, &urlErr) {
				t.Fatalf("error %v, want a url.Error", err)
			}
			decoded, _ := url.PathUnescape(urlErr.URL)
			if holdsToken(err.Error(), tc.token) || holdsToken(decoded, tc.token) {
				t.Errorf("error holds the token: %v", err)
			}
			if !strings.HasPrefix(urlErr.URL, srv.URL+apiPath+"/issues/") {
				t.Errorf("error URL %q, want the request's path", urlErr.URL)
			}
			if n := utf8.RuneCountInString(urlErr.URL); n > maxDetailChars+1 {
				t.Errorf("error URL has %d characters", n)
			}
			if got := innermost(err).Error(); got != io.EOF.Error() {
				t.Errorf("innermost error %q, want EOF", got)
			}
		})
	}
}

func TestRequestErrorsKeepTheirIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.Query().Has("cursor") {
			_, _ = fmt.Fprintf(w, `{"next":"http://x/p/?cursor=%s","previous":null,"results":[{"id":1,"name":"p"}]}`, echoedToken)
			return
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, echoedToken, &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = c.ListProjects(ctx)
	var urlErr *url.Error
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &urlErr) || !urlErr.Timeout() || holdsToken(err.Error(), echoedToken) {
		t.Fatalf("deadline: error %v, want a timed-out url.Error with context.DeadlineExceeded and no token", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	if _, err = c.ListProjects(ctx); !errors.Is(err, context.Canceled) || holdsToken(err.Error(), echoedToken) {
		t.Fatalf("cancel: error %v, want context.Canceled and no token", err)
	}
	c, err = New(srv.URL, echoedToken, &http.Client{Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ListProjects(context.Background()); !errors.As(err, &urlErr) || !urlErr.Timeout() || holdsToken(err.Error(), echoedToken) {
		t.Fatalf("client timeout: error %v, want a timed-out url.Error and no token", err)
	}
}

// chunkedOK is a chunked 200 answer up to its trailer section.
const chunkedOK = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n2\r\n{}\r\n0\r\n"

func innermost(err error) error {
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}
	return err
}

func TestBodyErrorsKeepTheirIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, echoedToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := c.GetIssue(ctx, "X-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v, want context.DeadlineExceeded", err)
	}
}
