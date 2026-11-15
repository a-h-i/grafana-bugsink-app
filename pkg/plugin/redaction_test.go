// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// hostileAnswer writes a refusal that holds the request's Authorization header in some form.
type hostileAnswer func(w http.ResponseWriter, auth string)

func jsonAnswer(status int, body func(auth string) string) hostileAnswer {
	return func(w http.ResponseWriter, auth string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body(auth))
	}
}

// rawAnswer writes text instead of an HTTP answer.
func rawAnswer(raw func(auth string) string) hostileAnswer {
	return func(w http.ResponseWriter, auth string) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString(raw(auth))
		_ = buf.Flush()
	}
}

func bare(auth string) string { return strings.TrimPrefix(auth, "Bearer ") }

type hostileCase struct {
	name   string
	token  string // empty for secretToken
	answer hostileAnswer
	shows  string // text every message shows
	onRead bool   // a 2xx answer that fails only a call that reads its body
}

func (c hostileCase) secret() string {
	if c.token == "" {
		return secretToken
	}
	return c.token
}

var hostileCases = []hostileCase{
	{name: "detail with Bearer", answer: jsonAnswer(400, func(a string) string { return `{"detail":"echo ` + a + `"}` }),
		shows: "Bugsink refused the request: echo Bearer [redacted]"},
	{name: "detail without Bearer", answer: jsonAnswer(400, func(a string) string { return `{"detail":"echo ` + bare(a) + `"}` }),
		shows: "Bugsink refused the request: echo [redacted]"},
	{name: "lower-case prefix", answer: jsonAnswer(400, func(a string) string { return `{"detail":"echo bearer ` + bare(a) + `"}` }),
		shows: "echo bearer [redacted]"},
	{name: "upper-case token", answer: jsonAnswer(400, func(a string) string { return `{"detail":"echo ` + strings.ToUpper(a) + `"}` }),
		shows: "echo BEARER [redacted]"},
	{name: "twice", answer: jsonAnswer(400, func(a string) string { return `{"detail":"` + a + ` ` + bare(a) + `"}` }),
		shows: "Bearer [redacted] [redacted]"},
	{name: "token across the cut", answer: jsonAnswer(400, func(a string) string { return `{"detail":"` + strings.Repeat("x", 495) + bare(a) + `"}` }),
		shows: "xxx[reda…"},
	{name: "list body", answer: jsonAnswer(400, func(a string) string { return `["echo","` + bare(a) + `"]` }),
		shows: "Bugsink refused the request: echo [redacted]"},
	{name: "field error", answer: jsonAnswer(400, func(a string) string { return `{"q":["` + bare(a) + `"]}` }),
		shows: "Bugsink refused the request: q: [redacted]"},
	{name: "html body", answer: func(w http.ResponseWriter, a string) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(400)
		_, _ = fmt.Fprint(w, "<p>"+a+"</p>")
	}, shows: "Bugsink answered HTTP 400."},
	{name: "capability", answer: jsonAnswer(403, func(a string) string {
		return `{"detail":"This token does not have the required capability: ` + bare(a) + `."}`
	}), shows: "The API token lacks [redacted]."},
	{name: "token inside words", token: "allow", answer: jsonAnswer(403, func(string) string {
		return `{"detail":"This token is not allowed to access this project."}`
	}), shows: "This token is not [redacted]ed to access this project."},
	{name: "detail at the read limit", answer: jsonAnswer(400, func(a string) string {
		return `{"detail":"` + strings.Repeat("x", 10<<20-len(`{"detail":""}`)-len(a)) + a + `"}`
	}), shows: "xxx…"},
	{name: "malformed answer", answer: rawAnswer(func(a string) string { return "HTTP/1.1 200 OK\r\n" + a + "\r\n\r\n" }),
		shows: "[redacted]"},
	{name: "trailer", answer: rawAnswer(func(a string) string { return chunkedOK + a + "\r\n\r\n" }),
		shows: "Bearer [redacted]", onRead: true},
	{name: "long trailer", answer: rawAnswer(func(string) string { return chunkedOK + strings.Repeat("x", 3000) + "\r\n\r\n" }),
		shows: "xxx…", onRead: true},
	{name: "unparsable Location", answer: func(w http.ResponseWriter, a string) {
		w.Header().Set("Location", "%zz"+a)
		w.WriteHeader(http.StatusFound)
	}, shows: "Bugsink answered HTTP 302."},
}

// chunkedOK is a chunked 200 answer up to its trailer section.
const chunkedOK = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n2\r\n{}\r\n0\r\n"

// maxHealthChars is the most characters a health message has: Bugsink's text cut to 500 plus our sentence and URL.
const maxHealthChars = 700

// assertNoSecret fails when text holds secret in any letter case.
func assertNoSecret(t *testing.T, what, text, secret string) {
	t.Helper()
	if strings.Contains(strings.ToLower(text), strings.ToLower(secret)) {
		t.Fatalf("%s leaks the token: %.300s", what, text)
	}
}

// hostileServer answers every request with answer; with commentsOnly, action POSTs get fakeBugsink's answer instead.
func hostileServer(t *testing.T, answer hostileAnswer, commentsOnly bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if commentsOnly && r.Method == http.MethodPost && r.URL.Path != commentsPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, issueAfter(r.URL.Path))
			return
		}
		answer(w, r.Header.Get("Authorization"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

var editor = &backend.User{Login: "ed", Role: "Editor"}

// failedQueryText is the error and frames of a query that must fail.
func failedQueryText(t *testing.T, r backend.DataResponse) string {
	t.Helper()
	if r.Error == nil {
		t.Fatalf("no error, %d frames", len(r.Frames))
	}
	return r.Error.Error() + fmt.Sprint(r.Frames)
}

func TestForwardedTextNeverHoldsToken(t *testing.T) {
	routes := []struct {
		name         string
		commentsOnly bool
		logs         bool // the route writes a log line
		run          func(t *testing.T, ds *Datasource) string
	}{
		{"issues query", false, false, func(t *testing.T, ds *Datasource) string {
			r := runQuery(t, ds, `{"queryType":"issues"}`)
			if r.Error == nil {
				t.Fatalf("no error, %d frames", len(r.Frames))
			}
			return r.Error.Error() + fmt.Sprint(r.Frames)
		}},
		{"issueCount query", false, false, func(t *testing.T, ds *Datasource) string {
			return failedQueryText(t, runQuery(t, ds, `{"queryType":"issueCount","project":1}`))
		}},
		{"eventCount query", false, false, func(t *testing.T, ds *Datasource) string {
			return failedQueryText(t, runQuery(t, ds, `{"queryType":"eventCount","project":1}`))
		}},
		{"newIssues query", false, false, func(t *testing.T, ds *Datasource) string {
			return failedQueryText(t, runQuery(t, ds, `{"queryType":"newIssues","project":1}`))
		}},
		{"GET issues/X-1", false, false, func(t *testing.T, ds *Datasource) string {
			r := callResource(t, ds, "GET", "issues/X-1")
			if r.status != http.StatusBadGateway {
				t.Fatalf("status %d", r.status)
			}
			return messageOf(t, r) + fmt.Sprint(r.headers)
		}},
		{"POST issues/X-1/resolve", false, true, func(t *testing.T, ds *Datasource) string {
			r := callAs(t, ds, editor, "POST", "issues/X-1/resolve", "")
			if r.status != http.StatusBadGateway {
				t.Fatalf("status %d", r.status)
			}
			return messageOf(t, r) + fmt.Sprint(r.headers)
		}},
		{"comment after POST issues/X-1/resolve", true, true, func(t *testing.T, ds *Datasource) string {
			r := callAs(t, ds, editor, "POST", "issues/X-1/resolve", "")
			if r.status != http.StatusOK || !strings.Contains(r.body, `"warning"`) {
				t.Fatalf("status %d body %.300s", r.status, r.body)
			}
			return r.body + fmt.Sprint(r.headers)
		}},
	}
	for _, c := range hostileCases {
		for _, route := range routes {
			if c.onRead && route.commentsOnly {
				continue
			}
			t.Run(c.name+"/"+route.name, func(t *testing.T) {
				logs := captureLogs(t)
				srv := hostileServer(t, c.answer, route.commentsOnly)
				text := route.run(t, newInstance(t, srv.URL, c.secret(), triageOn))
				assertNoSecret(t, "message", text, c.secret())
				if !strings.Contains(text, c.shows) {
					t.Fatalf("message %.300q does not show %q", text, c.shows)
				}
				if route.logs && len(logs.all()) == 0 {
					t.Fatal("no log line")
				}
				for _, l := range logs.all() {
					assertNoSecret(t, "log line", l.String(), c.secret())
				}
			})
		}
	}
}

func TestHealthMessagesNeverHoldToken(t *testing.T) {
	for _, c := range hostileCases {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLogs(t)
			srv := hostileServer(t, c.answer, false)
			res := check(t, newInstance(t, srv.URL, c.secret(), triageOn))
			if res.Status != backend.HealthStatusError {
				t.Fatalf("status %v %q", res.Status, res.Message)
			}
			assertNoSecret(t, "health message", res.Message+string(res.JSONDetails), c.secret())
			if n := utf8.RuneCountInString(res.Message); n > maxHealthChars {
				t.Fatalf("health message has %d characters: %.300s", n, res.Message)
			}
			for _, l := range logs.all() {
				assertNoSecret(t, "log line", l.String(), c.secret())
			}
		})
	}
}
