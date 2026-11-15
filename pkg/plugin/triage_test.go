// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

var triageActions = []string{"resolve", "resolve-latest", "resolve-next", "reopen", "mute", "mute-for", "mute-until", "unmute"}

var bodilessActions = []string{"resolve", "resolve-latest", "resolve-next", "reopen", "mute", "unmute"}

const (
	triageOn        = `{"browserUrl":"http://b","triageEnabled":true}`
	triageNoComment = `{"browserUrl":"http://b","triageEnabled":true,"attributionComments":false}`
	commentsPath    = "/api/canonical/0/issue-comments/"
	hostileBody     = `{"period":"day","periods":"2","__proto__":{},"comment":"by mallory"}` + "\x00"
)

// validBodies are bodies the mute routes accept.
var validBodies = map[string]string{
	"mute-for":   `{"period":"day","periods":2}`,
	"mute-until": `{"period":"hour","periods":1,"threshold":10}`,
}

// sentTerms are the bodies Bugsink receives for validBodies.
var sentTerms = map[string]map[string]any{
	"mute-for":   {"period_name": "day", "nr_of_periods": float64(2)},
	"mute-until": {"period_name": "hour", "nr_of_periods": float64(1), "gte_threshold": float64(10)},
}

// actionTexts are the attribution texts of each action with validBodies, without the author.
var actionTexts = map[string]string{
	"resolve":        "Resolved",
	"resolve-latest": "Resolved in the latest release",
	"resolve-next":   "Resolved in the next release",
	"reopen":         "Reopened",
	"mute":           "Muted",
	"mute-for":       "Muted for 2 days",
	"mute-until":     "Muted until 10 or more events in 1 hour",
	"unmute":         "Unmuted",
}

// triageBugsink serves fakeBugsink's reads, answers every POST, and records every request.
type triageBugsink struct {
	fake          *fakeBugsink
	rec           *recordingServer
	mu            sync.Mutex
	status        int           // status of every action POST, 0 for 200 with the issue in the state the action leads to
	body          string        // body of every action POST when status is set
	commentStatus int           // status of every comment POST, 0 for 201 with the comment
	commentBody   string        // body of every comment POST when commentStatus is set
	hangUp        bool          // close the connection on every comment POST without an answer
	held          chan struct{} // when set, the next issue list read reports here and waits for release
	release       chan struct{}
}

func newTriageBugsink(t *testing.T) *triageBugsink {
	t.Helper()
	tb := &triageBugsink{fake: newFakeBugsink(t)}
	tb.rec = newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			if r.URL.Path == "/api/canonical/0/issues/" {
				tb.mu.Lock()
				held, release := tb.held, tb.release
				tb.held = nil
				tb.mu.Unlock()
				if held != nil {
					held <- struct{}{}
					<-release
				}
			}
			tb.fake.serve(w, r)
			return
		}
		if r.URL.Path == commentsPath {
			tb.serveComment(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		tb.mu.Lock()
		status, body := tb.status, tb.body
		tb.mu.Unlock()
		if status == 0 {
			status, body = http.StatusOK, issueAfter(r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	})
	return tb
}

func (tb *triageBugsink) serveComment(w http.ResponseWriter) {
	tb.mu.Lock()
	status, body, hangUp := tb.commentStatus, tb.commentBody, tb.hangUp
	tb.mu.Unlock()
	if hangUp {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	if status == 0 {
		status = http.StatusCreated
		body = `{"id":373,"issue":"` + defaultIssueUUID + `","project":1,"timestamp":"2026-05-20T00:00:00Z","comment":"x","user":null}`
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprint(w, body)
}

// issueAfter is the issue Bugsink answers a successful triage POST to path with.
func issueAfter(path string) string {
	issue := issueAt("SAMPLE-PYTHON-7", rangeTo)
	switch {
	case strings.HasSuffix(path, "/resolve/"), strings.HasSuffix(path, "/resolve-latest/"):
		return strings.Replace(issue, `"is_resolved":false`, `"is_resolved":true`, 1)
	case strings.HasSuffix(path, "/resolve-next/"):
		issue = strings.Replace(issue, `"is_resolved":false`, `"is_resolved":true`, 1)
		return strings.Replace(issue, `"is_resolved_by_next_release":false`, `"is_resolved_by_next_release":true`, 1)
	case strings.HasSuffix(path, "/mute/"), strings.HasSuffix(path, "/mute-for/"), strings.HasSuffix(path, "/mute-until/"):
		return strings.Replace(issue, `"is_muted":false`, `"is_muted":true`, 1)
	default:
		return issue
	}
}

func (tb *triageBugsink) answer(status int, body string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.status, tb.body = status, body
}

func (tb *triageBugsink) answerComment(status int, body string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.commentStatus, tb.commentBody = status, body
}

func (tb *triageBugsink) hangUpOnComments() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.hangUp = true
}

// holdNextList makes the next issue list read wait: it reports on the first channel and answers when the second is closed.
func (tb *triageBugsink) holdNextList() (<-chan struct{}, chan<- struct{}) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.held, tb.release = make(chan struct{}), make(chan struct{})
	return tb.held, tb.release
}

func (tb *triageBugsink) datasource(t *testing.T, jsonData string) *Datasource {
	return newInstance(t, tb.rec.srv.URL, secretToken, jsonData)
}

func (tb *triageBugsink) posts() []seenRequest {
	var out []seenRequest
	for _, s := range tb.rec.requests() {
		if s.method == http.MethodPost {
			out = append(out, s)
		}
	}
	return out
}

func (tb *triageBugsink) actionPosts() []seenRequest {
	var out []seenRequest
	for _, s := range tb.posts() {
		if s.path != commentsPath {
			out = append(out, s)
		}
	}
	return out
}

func (tb *triageBugsink) commentPosts() []seenRequest {
	var out []seenRequest
	for _, s := range tb.posts() {
		if s.path == commentsPath {
			out = append(out, s)
		}
	}
	return out
}

func (tb *triageBugsink) assertNoRequests(t *testing.T) {
	t.Helper()
	if seen := tb.rec.requests(); len(seen) != 0 {
		t.Fatalf("Bugsink saw %d requests: %+v", len(seen), seen)
	}
}

// assertComment checks that Bugsink got exactly one comment, on the issue's uuid with text.
func (tb *triageBugsink) assertComment(t *testing.T, text string) {
	t.Helper()
	comments := tb.commentPosts()
	if len(comments) != 1 {
		t.Fatalf("Bugsink got %d comments: %+v", len(comments), comments)
	}
	assertJSONBody(t, comments[0].body, map[string]any{"issue": defaultIssueUUID, "comment": text})
}

// assertJSONBody checks that body decodes to exactly want.
func assertJSONBody(t *testing.T, body string, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("body %s, want %v (err %v)", body, want, err)
	}
}

// bodyFor is the valid body of action, empty for the bodiless actions.
func bodyFor(action string) string { return validBodies[action] }

// callAs sends a resource call as user with the triage header the app sends.
func callAs(t *testing.T, d *Datasource, user *backend.User, method, path, body string) resourceReply {
	t.Helper()
	return callWithHeaders(t, d, user, method, path, body, map[string][]string{"X-Bugsink-Triage": {"1"}})
}

// callWithHeaders sends a resource call as user with headers; a query string in path goes into the request URL.
func callWithHeaders(t *testing.T, d *Datasource, user *backend.User, method, path, body string, headers map[string][]string) resourceReply {
	t.Helper()
	req := &backend.CallResourceRequest{
		PluginContext: backend.PluginContext{User: user},
		Method:        method,
		Path:          path,
		Body:          []byte(body),
		Headers:       headers,
	}
	if p, query, ok := strings.Cut(path, "?"); ok {
		req.Path, req.URL = p, "/api/datasources/uid/bugsink/resources/"+p+"?"+query
	}
	sender := &collectingSender{}
	if err := d.CallResource(context.Background(), req, sender); err != nil {
		t.Fatalf("CallResource returned error: %v", err)
	}
	if sender.reply == nil {
		t.Fatal("no response sent")
	}
	return *sender.reply
}

func header(r resourceReply, name string) string {
	if v := r.headers[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func assertNoStore(t *testing.T, r resourceReply) {
	t.Helper()
	if got := header(r, "Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control %q, want no-store (status %d)", got, r.status)
	}
}

func assertReplyHasNoToken(t *testing.T, r resourceReply) {
	t.Helper()
	if strings.Contains(r.body, secretToken) || strings.Contains(fmt.Sprint(r.headers), secretToken) {
		t.Fatalf("reply leaks the token: %d %v %s", r.status, r.headers, r.body)
	}
}

type logLine struct {
	level, msg string
	args       []any
}

func (l logLine) String() string { return fmt.Sprint(l.level, " ", l.msg, " ", l.args) }

// logCapture replaces backend.Logger for one test and keeps every line.
type logCapture struct {
	mu    sync.Mutex
	lines []logLine
}

func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	prev := backend.Logger
	backend.Logger = capturingLogger{c: c}
	t.Cleanup(func() { backend.Logger = prev })
	return c
}

func (c *logCapture) all() []logLine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]logLine(nil), c.lines...)
}

func (c *logCapture) assertNoToken(t *testing.T) {
	t.Helper()
	for _, l := range c.all() {
		if strings.Contains(l.String(), secretToken) {
			t.Fatalf("log line leaks the token: %s", l)
		}
	}
}

// triageLines returns the "Triage" lines as key-value maps.
func (c *logCapture) triageLines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range c.all() {
		if l.msg != "Triage" {
			continue
		}
		if l.level != "info" || len(l.args)%2 != 0 {
			t.Fatalf("triage line %s", l)
		}
		m := map[string]any{}
		for i := 0; i < len(l.args); i += 2 {
			m[fmt.Sprint(l.args[i])] = l.args[i+1]
		}
		out = append(out, m)
	}
	return out
}

// assertTriageLine checks that exactly one triage line was written and that it is want.
func (c *logCapture) assertTriageLine(t *testing.T, want map[string]any) {
	t.Helper()
	if lines := c.triageLines(t); len(lines) != 1 || !reflect.DeepEqual(lines[0], want) {
		t.Fatalf("triage log lines %v, want one %v", lines, want)
	}
}

type capturingLogger struct {
	c    *logCapture
	with []any
}

func (l capturingLogger) add(level, msg string, args []any) {
	l.c.mu.Lock()
	defer l.c.mu.Unlock()
	l.c.lines = append(l.c.lines, logLine{level: level, msg: msg, args: append(append([]any(nil), l.with...), args...)})
}

func (l capturingLogger) Debug(msg string, args ...any) { l.add("debug", msg, args) }
func (l capturingLogger) Info(msg string, args ...any)  { l.add("info", msg, args) }
func (l capturingLogger) Warn(msg string, args ...any)  { l.add("warn", msg, args) }
func (l capturingLogger) Error(msg string, args ...any) { l.add("error", msg, args) }
func (l capturingLogger) Level() log.Level              { return log.Debug }
func (l capturingLogger) FromContext(context.Context) log.Logger {
	return l
}
func (l capturingLogger) With(args ...any) log.Logger {
	return capturingLogger{c: l.c, with: append(append([]any(nil), l.with...), args...)}
}

func TestTriageRoutes(t *testing.T) {
	wantStatus := map[string]string{
		"resolve": "resolved", "resolve-latest": "resolved", "resolve-next": "resolved", "reopen": "open",
		"mute": "muted", "mute-for": "muted", "mute-until": "muted", "unmute": "open",
	}
	for _, user := range []*backend.User{{Login: "ed", Role: "Editor"}, {Login: "ad", Role: "Admin"}} {
		for _, attribution := range []bool{true, false} {
			for _, action := range triageActions {
				t.Run(fmt.Sprintf("%s/attribution %v/%s", user.Role, attribution, action), func(t *testing.T) {
					logs := captureLogs(t)
					tb := newTriageBugsink(t)
					jsonData := triageOn
					if !attribution {
						jsonData = triageNoComment
					}
					r := callAs(t, tb.datasource(t, jsonData), user, "POST", "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action))
					if r.status != 200 {
						t.Fatalf("status %d body %s", r.status, r.body)
					}
					var body map[string]any
					if err := json.Unmarshal([]byte(r.body), &body); err != nil {
						t.Fatal(err)
					}
					if body["status"] != wantStatus[action] || body["project"] != "alpha" || body["friendlyId"] != "SAMPLE-PYTHON-7" {
						t.Fatalf("body %s", r.body)
					}
					if _, ok := body["warning"]; ok {
						t.Fatalf("warning in %s", r.body)
					}
					posts := tb.posts()
					if len(posts) == 0 || posts[0].path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/"+action+"/" {
						t.Fatalf("Bugsink POSTs %+v", posts)
					}
					if want, ok := sentTerms[action]; ok {
						assertJSONBody(t, posts[0].body, want)
					} else if posts[0].body != "" {
						t.Fatalf("Bugsink got body %q", posts[0].body)
					}
					want := map[string]any{"action": action, "issue": "SAMPLE-PYTHON-7", "user": user.Login, "role": user.Role, "status": 200}
					if attribution {
						if len(posts) != 2 {
							t.Fatalf("Bugsink POSTs %+v", posts)
						}
						tb.assertComment(t, actionTexts[action]+" from Grafana by \""+user.Login+"\".")
						want["comment"] = 201
					} else if len(posts) != 1 {
						t.Fatalf("Bugsink POSTs %+v", posts)
					}
					logs.assertTriageLine(t, want)
					assertReplyHasNoToken(t, r)
					logs.assertNoToken(t)
				})
			}
		}
	}
}

func TestTriageRefusesCaller(t *testing.T) {
	users := []struct {
		name string
		user *backend.User
	}{
		{"no user", nil},
		{"empty role", &backend.User{}},
		{"empty role with a login", &backend.User{Login: "someone"}},
		{"None", &backend.User{Role: "None"}},
		{"Viewer", &backend.User{Role: "Viewer"}},
		{"lower case editor", &backend.User{Role: "editor"}},
		{"upper case admin", &backend.User{Role: "ADMIN"}},
		{"editor with surrounding spaces", &backend.User{Role: " Editor "}},
		{"admin with a trailing newline", &backend.User{Role: "Admin\n"}},
		{"server admin name", &backend.User{Role: "GrafanaAdmin"}},
		{"login admin with role Viewer", &backend.User{Login: "admin", Role: "Viewer"}},
	}
	settings := []struct{ name, jsonData string }{
		{"triage on", triageOn},
		{"triage off", `{"browserUrl":"http://b"}`},
		{"invalid settings", `{"triageEnabled":"yes"}`},
	}
	for _, u := range users {
		for _, s := range settings {
			for _, action := range triageActions {
				t.Run(u.name+"/"+s.name+"/"+action, func(t *testing.T) {
					logs := captureLogs(t)
					tb := newTriageBugsink(t)
					r := callAs(t, tb.datasource(t, s.jsonData), u.user, "POST", "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action))
					if r.status != 403 || messageOf(t, r) != msgCallerRefused {
						t.Fatalf("status %d body %s", r.status, r.body)
					}
					assertNoStore(t, r)
					tb.assertNoRequests(t)
					if lines := logs.triageLines(t); len(lines) != 0 {
						t.Fatalf("triage log lines %v", lines)
					}
				})
			}
		}
	}
	for _, action := range triageActions {
		t.Run("refused before the id is read/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), &backend.User{Role: "Viewer"}, "POST", "issues/a.b/"+action, "")
			if r.status != 403 || messageOf(t, r) != msgCallerRefused {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
		t.Run("refused before the body is read/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), &backend.User{Role: "Viewer"}, "POST", "issues/SAMPLE-PYTHON-7/"+action, hostileBody)
			if r.status != 403 || messageOf(t, r) != msgCallerRefused {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
	}
}

func TestTriageMethods(t *testing.T) {
	users := []*backend.User{{Role: "Admin"}, {Role: "Viewer"}, nil}
	for _, user := range users {
		for _, method := range []string{"GET", "PUT", "PATCH", "DELETE", "HEAD"} {
			for _, action := range triageActions {
				t.Run(fmt.Sprintf("%v/%s/%s", user, method, action), func(t *testing.T) {
					tb := newTriageBugsink(t)
					r := callAs(t, tb.datasource(t, triageOn), user, method, "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action))
					if r.status != 405 || header(r, "Allow") != "POST" {
						t.Fatalf("status %d Allow %q body %s", r.status, header(r, "Allow"), r.body)
					}
					if method != "HEAD" && messageOf(t, r) != "Method not allowed." {
						t.Fatalf("body %s", r.body)
					}
					assertNoStore(t, r)
					tb.assertNoRequests(t)
				})
			}
		}
	}
}

func TestTriageOff(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range triageActions {
		for _, c := range []struct{ name, jsonData, path, body string }{
			{"switch absent", `{"browserUrl":"http://b"}`, "issues/SAMPLE-PYTHON-7/" + action, bodyFor(action)},
			{"switch off", `{"browserUrl":"http://b","triageEnabled":false}`, "issues/SAMPLE-PYTHON-7/" + action, bodyFor(action)},
			{"no json data", "", "issues/SAMPLE-PYTHON-7/" + action, bodyFor(action)},
			{"off before the id is read", `{"triageEnabled":false}`, "issues/a.b/" + action, bodyFor(action)},
			{"off before the body is read", `{"triageEnabled":false}`, "issues/SAMPLE-PYTHON-7/" + action, hostileBody},
		} {
			t.Run(c.name+"/"+action, func(t *testing.T) {
				tb := newTriageBugsink(t)
				r := callAs(t, tb.datasource(t, c.jsonData), editor, "POST", c.path, c.body)
				if r.status != 403 || messageOf(t, r) != msgTriageOff {
					t.Fatalf("status %d body %s", r.status, r.body)
				}
				tb.assertNoRequests(t)
			})
		}
	}
}

func TestTriageNeedsHeader(t *testing.T) {
	headers := []struct {
		name    string
		headers map[string][]string
	}{
		{"no header", nil},
		{"empty", map[string][]string{"X-Bugsink-Triage": {""}}},
		{"zero", map[string][]string{"X-Bugsink-Triage": {"0"}}},
		{"true", map[string][]string{"X-Bugsink-Triage": {"true"}}},
		{"other header", map[string][]string{"Content-Type": {"application/json"}}},
	}
	for _, user := range []*backend.User{{Login: "ed", Role: "Editor"}, {Login: "ad", Role: "Admin"}, {Role: "Admin"}} {
		for _, h := range headers {
			for _, action := range triageActions {
				t.Run(fmt.Sprintf("%s %q/%s/%s", user.Role, user.Login, h.name, action), func(t *testing.T) {
					logs := captureLogs(t)
					tb := newTriageBugsink(t)
					r := callWithHeaders(t, tb.datasource(t, triageOn), user, "POST", "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action), h.headers)
					if r.status != 403 || messageOf(t, r) != msgTriageHeader {
						t.Fatalf("status %d body %s", r.status, r.body)
					}
					assertNoStore(t, r)
					tb.assertNoRequests(t)
					if lines := logs.triageLines(t); len(lines) != 0 {
						t.Fatalf("triage log lines %v", lines)
					}
				})
			}
		}
	}
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range triageActions {
		for _, c := range []struct {
			name, jsonData, path, body string
			user                       *backend.User
			status                     int
			message                    string
		}{
			{"caller refused first", triageOn, "issues/SAMPLE-PYTHON-7/" + action, "", &backend.User{Role: "Viewer"}, 403, msgCallerRefused},
			{"switch checked first", `{"browserUrl":"http://b"}`, "issues/SAMPLE-PYTHON-7/" + action, "", editor, 403, msgTriageOff},
			{"settings checked first", `{"triageEnabled":"yes"}`, "issues/SAMPLE-PYTHON-7/" + action, "", editor, 400, msgSettingsInvalid},
			{"refused before the id is read", triageOn, "issues/a.b/" + action, "", editor, 403, msgTriageHeader},
			{"refused before the body is read", triageOn, "issues/SAMPLE-PYTHON-7/" + action, hostileBody, editor, 403, msgTriageHeader},
		} {
			t.Run(c.name+"/"+action, func(t *testing.T) {
				tb := newTriageBugsink(t)
				r := callWithHeaders(t, tb.datasource(t, c.jsonData), c.user, "POST", c.path, c.body, nil)
				if r.status != c.status || messageOf(t, r) != c.message {
					t.Fatalf("status %d body %s", r.status, r.body)
				}
				tb.assertNoRequests(t)
			})
		}
	}
}

// tripwire is a request body that records whether it was read.
type tripwire struct{ read bool }

func (b *tripwire) Read([]byte) (int, error) {
	b.read = true
	return 0, io.EOF
}

func TestTriageGateBeforeBody(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range triageActions {
		for _, c := range []struct {
			name, jsonData string
			user           *backend.User
			header         string
			path           string
			status         int
			message        string
		}{
			{"Viewer", triageOn, &backend.User{Login: "v", Role: "Viewer"}, "1", "SAMPLE-PYTHON-7", 403, msgCallerRefused},
			{"missing header", triageOn, editor, "", "SAMPLE-PYTHON-7", 403, msgTriageHeader},
			{"triage off", `{"browserUrl":"http://b"}`, editor, "1", "SAMPLE-PYTHON-7", 403, msgTriageOff},
			{"invalid id", triageOn, editor, "1", "a.b", 400, notAnIssueID("a.b")},
		} {
			t.Run(c.name+"/"+action, func(t *testing.T) {
				tb := newTriageBugsink(t)
				body := &tripwire{}
				req := httptest.NewRequestWithContext(backend.WithUser(context.Background(), c.user),
					http.MethodPost, "/issues/"+c.path+"/"+action, body)
				if c.header != "" {
					req.Header.Set(triageHeader, c.header)
				}
				w := httptest.NewRecorder()
				tb.datasource(t, c.jsonData).routes().ServeHTTP(w, req)
				r := resourceReply{status: w.Code, body: w.Body.String()}
				if r.status != c.status || messageOf(t, r) != c.message {
					t.Fatalf("status %d body %s", r.status, r.body)
				}
				if body.read {
					t.Fatal("the body was read before the refusal")
				}
				tb.assertNoRequests(t)
			})
		}
	}
}

func TestTriageSettingsInvalid(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range triageActions {
		for _, c := range []struct{ name, jsonData, path, message string }{
			{"switch not a boolean", `{"triageEnabled":"yes"}`, "issues/SAMPLE-PYTHON-7/" + action, msgSettingsInvalid},
			{"browser url", `{"triageEnabled":true,"browserUrl":"ftp://x"}`, "issues/SAMPLE-PYTHON-7/" + action, msgBrowserURLScheme},
			{"attribution not a boolean", `{"triageEnabled":true,"attributionComments":"no"}`, "issues/SAMPLE-PYTHON-7/" + action, msgSettingsInvalid},
			{"invalid before the id is read", `{"triageEnabled":"yes"}`, "issues/a.b/" + action, msgSettingsInvalid},
		} {
			t.Run(c.name+"/"+action, func(t *testing.T) {
				tb := newTriageBugsink(t)
				r := callAs(t, tb.datasource(t, c.jsonData), editor, "POST", c.path, bodyFor(action))
				if r.status != 400 || messageOf(t, r) != c.message {
					t.Fatalf("status %d body %s", r.status, r.body)
				}
				tb.assertNoRequests(t)
			})
		}
		t.Run("no url/"+action, func(t *testing.T) {
			r := callAs(t, newInstance(t, "", secretToken, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action))
			if r.status != 400 || messageOf(t, r) != msgEnterURL {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
		})
	}
}

func TestTriageRejectsIDs(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	long := strings.Repeat("a", 101)
	for _, action := range triageActions {
		for _, c := range []struct{ path, id string }{
			{"issues/../" + action, ".."},
			{"issues/./" + action, "."},
			{"issues/%2e%2e/" + action, ".."},
			{"issues/a%2Fb/" + action, "a/b"},
			{"issues/x%20y/" + action, "x y"},
			{"issues/x%3Fy/" + action, "x?y"},
			{"issues//" + action, ""},
			{"issues/a.b/" + action, "a.b"},
			{"issues/" + long + "/" + action, long},
		} {
			t.Run(c.path, func(t *testing.T) {
				tb := newTriageBugsink(t)
				r := callAs(t, tb.datasource(t, triageOn), editor, "POST", c.path, bodyFor(action))
				if r.status != 400 || messageOf(t, r) != notAnIssueID(c.id) {
					t.Fatalf("status %d body %s", r.status, r.body)
				}
				tb.assertNoRequests(t)
			})
		}
		t.Run("id checked before the body/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/a.b/"+action, hostileBody)
			if r.status != 400 || messageOf(t, r) != notAnIssueID("a.b") {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
		t.Run("100 characters is an id/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			id := strings.Repeat("a", 100)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/"+id+"/"+action, bodyFor(action))
			if r.status != 200 || len(tb.actionPosts()) != 1 {
				t.Fatalf("status %d body %s posts %+v", r.status, r.body, tb.posts())
			}
		})
	}
}

func TestTriageSendsOnlyTheAction(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range bodilessActions {
		t.Run(action, func(t *testing.T) {
			logs := captureLogs(t)
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/"+action+"?gte_threshold=5",
				`{"period_name":"day","nr_of_periods":2,"comment":"by mallory"}`)
			if r.status != 200 {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			posts := tb.actionPosts()
			if len(posts) != 1 {
				t.Fatalf("Bugsink POSTs %+v", tb.posts())
			}
			if p := posts[0]; p.path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/"+action+"/" || p.body != "" || p.rawQuery != "" {
				t.Fatalf("Bugsink saw %+v", p)
			}
			tb.assertComment(t, actionTexts[action]+` from Grafana by "ed".`)
			for _, l := range logs.all() {
				if s := l.String(); strings.Contains(s, "period_name") || strings.Contains(s, "gte_threshold") || strings.Contains(s, "mallory") {
					t.Fatalf("log line holds the request: %s", s)
				}
			}
		})
	}
}

func TestMuteSendsOnlyTerms(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, c := range []struct {
		action, body string
		want         map[string]any
	}{
		{"mute-for", `{"period":"week","periods":100}`, map[string]any{"period_name": "week", "nr_of_periods": float64(100)}},
		{"mute-until", `{"period":"minute","periods":1,"threshold":1000000}`,
			map[string]any{"period_name": "minute", "nr_of_periods": float64(1), "gte_threshold": float64(1000000)}},
	} {
		t.Run(c.action, func(t *testing.T) {
			logs := captureLogs(t)
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageNoComment), editor, "POST",
				"issues/SAMPLE-PYTHON-7/"+c.action+"?nr_of_periods=0&gte_threshold=0", c.body)
			if r.status != 200 {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			posts := tb.posts()
			if len(posts) != 1 || posts[0].rawQuery != "" || posts[0].path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/"+c.action+"/" {
				t.Fatalf("Bugsink POSTs %+v", posts)
			}
			assertJSONBody(t, posts[0].body, c.want)
			for _, l := range logs.all() {
				if s := l.String(); strings.Contains(s, "period") || strings.Contains(s, "threshold") {
					t.Fatalf("log line holds the body: %s", s)
				}
			}
		})
	}
}

func TestMuteAcceptsBodies(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	padded := `{"period":"day","periods":2}`
	padded += strings.Repeat(" ", 1024-len(padded))
	for _, c := range []struct{ name, action, body string }{
		{"keys in any order", "mute-until", `{"threshold":10,"periods":1,"period":"hour"}`},
		{"whitespace around", "mute-for", " \n\t{ \"period\" : \"day\" , \"periods\" : 2 }\r\n"},
		{"1024 bytes", "mute-for", padded},
		{"every period", "mute-for", `{"period":"minute","periods":1}`},
		{"100 periods", "mute-until", `{"period":"year","periods":100,"threshold":1}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageNoComment), editor, "POST", "issues/SAMPLE-PYTHON-7/"+c.action, c.body)
			if r.status != 200 || len(tb.posts()) != 1 {
				t.Fatalf("status %d body %s posts %+v", r.status, r.body, tb.posts())
			}
		})
	}
}

func TestMuteRejectsBodies(t *testing.T) {
	const (
		shapeFor     = "The request body must be a JSON object with period and periods."
		shapeUntil   = "The request body must be a JSON object with period, periods and threshold."
		periodMsg    = "The period must be minute, hour, day, week, month or year."
		periodsMsg   = "The number of periods must be a whole number from 1 to 100."
		thresholdMsg = "The threshold must be a whole number from 1 to 1000000."
		sizeMsg      = "The request body is larger than 1024 bytes."
	)
	editor := &backend.User{Login: "ed", Role: "Editor"}
	type bodyCase struct{ name, body, message string }
	for _, route := range []struct{ action, extra, shape string }{
		{"mute-for", "", shapeFor},
		{"mute-until", `,"threshold":5`, shapeUntil},
	} {
		obj := func(fields string) string { return "{" + fields + route.extra + "}" }
		oversized := obj(`"period":"day","periods":2`)
		oversized += strings.Repeat(" ", 1025-len(oversized))
		cases := []bodyCase{
			{"empty", "", route.shape},
			{"whitespace only", "  \n", route.shape},
			{"array", `[1]`, route.shape},
			{"string", `"x"`, route.shape},
			{"null", `null`, route.shape},
			{"number", `2`, route.shape},
			{"not JSON", `period=day&periods=2`, route.shape},
			{"cut short", obj(`"period":"day","periods":2`)[:10], route.shape},
			{"second value", obj(`"period":"day","periods":2`) + `{"x":1}`, route.shape},
			{"trailing text", obj(`"period":"day","periods":2`) + `x`, route.shape},
			{"trailing brace", obj(`"period":"day","periods":2`) + `}`, route.shape},
			{"unknown field", obj(`"period":"day","periods":2,"__proto__":{}`), route.shape},
			{"field in another case", obj(`"Period":"day","periods":2`), route.shape},
			{"duplicate field", obj(`"period":"day","period":"week","periods":2`), route.shape},
			{"duplicate number", obj(`"period":"day","periods":2,"periods":3`), route.shape},
			{"string number", obj(`"period":"day","periods":"2"`), route.shape},
			{"fraction", obj(`"period":"day","periods":2.5`), route.shape},
			{"whole fraction", obj(`"period":"day","periods":2.0`), route.shape},
			{"exponent", obj(`"period":"day","periods":1e1`), route.shape},
			{"null number", obj(`"period":"day","periods":null`), route.shape},
			{"boolean number", obj(`"period":"day","periods":true`), route.shape},
			{"object number", obj(`"period":"day","periods":{}`), route.shape},
			{"missing periods", obj(`"period":"day"`), route.shape},
			{"missing period", obj(`"periods":2`), route.shape},
			{"null period", obj(`"period":null,"periods":2`), route.shape},
			{"number period", obj(`"period":1,"periods":2`), route.shape},
			{"capitalised period", obj(`"period":"Day","periods":2`), periodMsg},
			{"unknown period", obj(`"period":"fortnight","periods":2`), periodMsg},
			{"plural period", obj(`"period":"days","periods":2`), periodMsg},
			{"empty period", obj(`"period":"","periods":2`), periodMsg},
			{"zero periods", obj(`"period":"day","periods":0`), periodsMsg},
			{"negative zero periods", obj(`"period":"day","periods":-0`), periodsMsg},
			{"negative periods", obj(`"period":"day","periods":-1`), periodsMsg},
			{"101 periods", obj(`"period":"day","periods":101`), periodsMsg},
			{"overflowing periods", obj(`"period":"day","periods":99999999999999999999999`), periodsMsg},
			{"1025 bytes", oversized, sizeMsg},
			{"1025 bytes of anything", strings.Repeat("x", 1025), sizeMsg},
		}
		if route.action == "mute-for" {
			cases = append(cases, bodyCase{"threshold on mute-for", `{"period":"day","periods":2,"threshold":5}`, shapeFor})
		} else {
			cases = append(cases,
				bodyCase{"missing threshold", `{"period":"day","periods":2}`, shapeUntil},
				bodyCase{"zero threshold", `{"period":"day","periods":2,"threshold":0}`, thresholdMsg},
				bodyCase{"negative threshold", `{"period":"day","periods":2,"threshold":-1}`, thresholdMsg},
				bodyCase{"threshold over the limit", `{"period":"day","periods":2,"threshold":1000001}`, thresholdMsg},
				bodyCase{"string threshold", `{"period":"day","periods":2,"threshold":"5"}`, shapeUntil},
				bodyCase{"fraction threshold", `{"period":"day","periods":2,"threshold":5.5}`, shapeUntil},
				bodyCase{"null threshold", `{"period":"day","periods":2,"threshold":null}`, shapeUntil},
			)
		}
		for _, c := range cases {
			t.Run(route.action+"/"+c.name, func(t *testing.T) {
				logs := captureLogs(t)
				tb := newTriageBugsink(t)
				r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/"+route.action, c.body)
				if r.status != 400 || messageOf(t, r) != c.message {
					t.Fatalf("status %d body %s, want 400 %q", r.status, r.body, c.message)
				}
				assertNoStore(t, r)
				tb.assertNoRequests(t)
				if lines := logs.triageLines(t); len(lines) != 0 {
					t.Fatalf("triage log lines %v", lines)
				}
			})
		}
	}
}

func TestTriageBugsinkErrors(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	cases := []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{"changed meanwhile, list", 400, `["Issue is already resolved."]`, "Bugsink refused the request: Issue is already resolved."},
		{"changed meanwhile, detail", 400, `{"detail":"Issue is already resolved."}`, "Bugsink refused the request: Issue is already resolved."},
		{"capability", 403, `{"detail":"This token does not have the required capability: issues:triage."}`,
			"The API token lacks issues:triage."},
		{"absent", 404, `{"detail":"No Issue matches the given query."}`, "Bugsink refused the request: No Issue matches the given query."},
		{"token rejected", 401, `{"detail":"Invalid Bearer token."}`, "Bugsink rejected the API token."},
		{"server error", 500, ``, "Bugsink answered HTTP 500."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLogs(t)
			tb := newTriageBugsink(t)
			tb.answer(c.status, c.body)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
			if r.status != 502 || messageOf(t, r) != c.message {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			if comments := tb.commentPosts(); len(comments) != 0 {
				t.Fatalf("comment after a refused action: %+v", comments)
			}
			logs.assertTriageLine(t, map[string]any{"action": "resolve", "issue": "SAMPLE-PYTHON-7", "user": "ed", "role": "Editor", "status": c.status})
			assertNoStore(t, r)
			assertReplyHasNoToken(t, r)
			logs.assertNoToken(t)
		})
	}

	t.Run("resolve in the latest release without a release", func(t *testing.T) {
		logs := captureLogs(t)
		tb := newTriageBugsink(t)
		tb.answer(400, `["Project has no releases."]`)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve-latest", "")
		if r.status != 502 || messageOf(t, r) != "Bugsink refused the request: Project has no releases." {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		if comments := tb.commentPosts(); len(comments) != 0 {
			t.Fatalf("comment after a refused action: %+v", comments)
		}
		logs.assertTriageLine(t, map[string]any{"action": "resolve-latest", "issue": "SAMPLE-PYTHON-7", "user": "ed", "role": "Editor", "status": 400})
	})

	t.Run("closed server", func(t *testing.T) {
		logs := captureLogs(t)
		ds := newInstance(t, closedPortURL(t), secretToken, triageOn)
		r := callAs(t, ds, editor, "POST", "issues/SAMPLE-PYTHON-7/unmute", "")
		if r.status != 502 || !strings.HasPrefix(messageOf(t, r), "Cannot reach Bugsink: ") {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		logs.assertTriageLine(t, map[string]any{"action": "unmute", "issue": "SAMPLE-PYTHON-7", "user": "ed", "role": "Editor", "status": 0})
		assertNoStore(t, r)
		assertReplyHasNoToken(t, r)
		logs.assertNoToken(t)
	})
}

func TestAttributionComment(t *testing.T) {
	editor := &backend.User{Login: "m7-editor", Role: "Editor"}
	t.Run("after the action", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/mute-for", `{"period":"day","periods":2}`)
		if r.status != 200 {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		posts := tb.posts()
		if len(posts) != 2 || posts[0].path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/mute-for/" || posts[1].path != commentsPath {
			t.Fatalf("Bugsink POSTs %+v", posts)
		}
		tb.assertComment(t, `Muted for 2 days from Grafana by "m7-editor".`)
	})
	t.Run("nothing from the request", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callWithHeaders(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve?login=mallory&comment=x",
			`{"login":"mallory","comment":"by mallory"}`, map[string][]string{
				"X-Bugsink-Triage": {"1"}, "X-Grafana-User": {"mallory"}, "X-Grafana-Login": {"mallory"},
			})
		if r.status != 200 {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		tb.assertComment(t, `Resolved from Grafana by "m7-editor".`)
	})
	t.Run("logs Bugsink's status for the comment", func(t *testing.T) {
		logs := captureLogs(t)
		tb := newTriageBugsink(t)
		tb.answerComment(200, `{}`)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 200 || strings.Contains(r.body, "warning") {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		logs.assertTriageLine(t, map[string]any{
			"action": "resolve", "issue": "SAMPLE-PYTHON-7", "user": "m7-editor", "role": "Editor", "status": 200, "comment": 200,
		})
	})
	t.Run("switch off", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageNoComment), editor, "POST", "issues/SAMPLE-PYTHON-7/mute-for", `{"period":"day","periods":2}`)
		if r.status != 200 || len(tb.actionPosts()) != 1 || len(tb.commentPosts()) != 0 {
			t.Fatalf("status %d posts %+v", r.status, tb.posts())
		}
	})
	t.Run("action refused", func(t *testing.T) {
		tb := newTriageBugsink(t)
		tb.answer(400, `["Issue is already muted."]`)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/mute-for", `{"period":"day","periods":2}`)
		if r.status != 502 || len(tb.actionPosts()) != 1 || len(tb.commentPosts()) != 0 {
			t.Fatalf("status %d posts %+v", r.status, tb.posts())
		}
	})
}

func TestAttributionCommentLogins(t *testing.T) {
	for _, c := range []struct{ name, login, who string }{
		{"control characters", "m7\x00ed\x1b[31mitor\t\r", "\"m7\uFFFDed\uFFFD[31mitor\uFFFD\uFFFD\""},
		{"newline", "m7\neditor", "\"m7\uFFFDeditor\""},
		{"forged second line", "ed.\r\nResolved from Grafana by admin", "\"ed.\uFFFD\uFFFDResolved from Grafana by admin\""},
		{"forged second claim", "ed. Resolved from Grafana by admin", "\"ed. Resolved from Grafana by admin\""},
		{"double quotes", "ed\". Resolved from Grafana by \"admin", "\"ed'. Resolved from Grafana by 'admin\""},
		{"typographic quotes", "ed\u201D Resolved from Grafana by \u201Cadmin", "\"ed' Resolved from Grafana by 'admin\""},
		{"other quotation marks", "\u00ABa\u00BB\u2039b\u203A\u300Cc\u300F\uFF02\u201E\u201F\u2018\u2019'", "\"'a''b''c'''''''\""},
		{"unicode separators", "a\u2028b\u2029c\u0085d", "\"a\uFFFDb\uFFFDc\uFFFDd\""},
		{"format characters", "ad\u202Emin\u200B", "\"ad\uFFFDmin\uFFFD\""},
		{"invalid UTF-8", "ed\xff", "\"ed\uFFFD\""},
		{"Markdown", "**ed** [x](http://e) `c` # h", "\"**ed** [x](http://e) `c` # h\""},
		{"HTML", "<b>x</b><script>alert(1)</script>&amp;", "\"<b>x</b><script>alert(1)</script>&amp;\""},
		{"very long", strings.Repeat("x", 150), "\"" + strings.Repeat("x", 100) + "\u2026\""},
		{"long multibyte", strings.Repeat("\u00E9", 101), "\"" + strings.Repeat("\u00E9", 100) + "\u2026\""},
		{"long with quotes", strings.Repeat("\"", 101), "\"" + strings.Repeat("'", 100) + "\u2026\""},
		{"exactly 100", strings.Repeat("y", 100), "\"" + strings.Repeat("y", 100) + "\""},
		{"empty", "", "an anonymous user"},
		{"template", "{{.Login}} ${user} %s", "\"{{.Login}} ${user} %s\""},
		{"mention", "@admin", "\"@admin\""},
		{"service account", "sa-1-triage", "\"sa-1-triage\""},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), &backend.User{Login: c.login, Role: "Admin"}, "POST", "issues/SAMPLE-PYTHON-7/unmute", "")
			if r.status != 200 {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertComment(t, "Unmuted from Grafana by "+c.who+".")
		})
	}
}

func TestAttributionCommentBytes(t *testing.T) {
	for _, c := range []struct{ name, login, body string }{
		{"invalid UTF-8", "ed\xff\xc3(\xe2\x82",
			"{\"comment\":\"Unmuted from Grafana by \\\"ed\xef\xbf\xbd\xef\xbf\xbd(\xef\xbf\xbd\xef\xbf\xbd\\\".\",\"issue\":\"" + defaultIssueUUID + "\"}"},
		{"bidirectional controls", "ad\xe2\x80\xaemin\xe2\x81\xa6x\xe2\x81\xa9\xe2\x80\x8f\xe2\x80\x8e\xe2\x80\xad",
			"{\"comment\":\"Unmuted from Grafana by \\\"ad\xef\xbf\xbdmin\xef\xbf\xbdx\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\\\".\",\"issue\":\"" + defaultIssueUUID + "\"}"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), &backend.User{Login: c.login, Role: "Editor"}, "POST", "issues/SAMPLE-PYTHON-7/unmute", "")
			if r.status != 200 {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			comments := tb.commentPosts()
			if len(comments) != 1 || comments[0].body != c.body {
				t.Fatalf("Bugsink got %+v\nwant body % x", comments, c.body)
			}
		})
	}
}

func TestAttributionText(t *testing.T) {
	day2 := bugsink.MuteTerms{Period: bugsink.PeriodDay, Periods: 2}
	for _, c := range []struct {
		action bugsink.TriageAction
		terms  bugsink.MuteTerms
		login  string
		want   string
	}{
		{bugsink.Resolve, bugsink.MuteTerms{}, "a", "Resolved from Grafana by \"a\"."},
		{bugsink.ResolveLatest, bugsink.MuteTerms{}, "a", "Resolved in the latest release from Grafana by \"a\"."},
		{bugsink.ResolveNext, bugsink.MuteTerms{}, "a", "Resolved in the next release from Grafana by \"a\"."},
		{bugsink.Reopen, bugsink.MuteTerms{}, "a", "Reopened from Grafana by \"a\"."},
		{bugsink.Mute, bugsink.MuteTerms{}, "a", "Muted from Grafana by \"a\"."},
		{bugsink.Unmute, bugsink.MuteTerms{}, "a", "Unmuted from Grafana by \"a\"."},
		{bugsink.MuteFor, day2, "a", "Muted for 2 days from Grafana by \"a\"."},
		{bugsink.MuteFor, bugsink.MuteTerms{Period: bugsink.PeriodHour, Periods: 1}, "a", "Muted for 1 hour from Grafana by \"a\"."},
		{bugsink.MuteFor, bugsink.MuteTerms{Period: bugsink.PeriodMinute, Periods: 100}, "a", "Muted for 100 minutes from Grafana by \"a\"."},
		{bugsink.MuteFor, bugsink.MuteTerms{Period: bugsink.PeriodWeek, Periods: 3}, "a", "Muted for 3 weeks from Grafana by \"a\"."},
		{bugsink.MuteFor, bugsink.MuteTerms{Period: bugsink.PeriodMonth, Periods: 1}, "a", "Muted for 1 month from Grafana by \"a\"."},
		{bugsink.MuteFor, bugsink.MuteTerms{Period: bugsink.PeriodYear, Periods: 5}, "a", "Muted for 5 years from Grafana by \"a\"."},
		{bugsink.MuteUntil, bugsink.MuteTerms{Period: bugsink.PeriodHour, Periods: 1, Threshold: 10}, "a",
			"Muted until 10 or more events in 1 hour from Grafana by \"a\"."},
		{bugsink.MuteUntil, bugsink.MuteTerms{Period: bugsink.PeriodDay, Periods: 7, Threshold: 1}, "a",
			"Muted until 1 or more events in 7 days from Grafana by \"a\"."},
		{bugsink.Resolve, bugsink.MuteTerms{}, "", "Resolved from Grafana by an anonymous user."},
		{bugsink.Resolve, bugsink.MuteTerms{}, "m7\neditor", "Resolved from Grafana by \"m7\uFFFDeditor\"."},
		{bugsink.Resolve, bugsink.MuteTerms{}, strings.Repeat("x", 150), "Resolved from Grafana by \"" + strings.Repeat("x", 100) + "\u2026\"."},
		{bugsink.Resolve, bugsink.MuteTerms{}, "<b>x</b>", "Resolved from Grafana by \"<b>x</b>\"."},
	} {
		t.Run(c.want, func(t *testing.T) {
			got := attributionText(c.action, c.terms, c.login)
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			if strings.ContainsAny(got, "\r\n  ") {
				t.Fatalf("text %q holds a line break", got)
			}
		})
	}
}

func TestAttributionFailureWarns(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	const prefix = "The action succeeded, but the attribution comment was not written: "
	for _, c := range []struct {
		name    string
		setup   func(tb *triageBugsink)
		warning string // exact warning, or a prefix when it ends in a colon
		comment int
	}{
		{"token lacks issues:comment", func(tb *triageBugsink) {
			tb.answerComment(403, `{"detail":"This token does not have the required capability: issues:comment."}`)
		}, prefix + "The API token lacks issues:comment.", 403},
		{"comment refused", func(tb *triageBugsink) {
			tb.answerComment(400, `{"comment":["This field may not be blank."]}`)
		}, prefix + "Bugsink refused the request: comment: This field may not be blank.", 400},
		{"server error", func(tb *triageBugsink) { tb.answerComment(500, "") }, prefix + "Bugsink answered HTTP 500.", 500},
		{"connection closed", func(tb *triageBugsink) { tb.hangUpOnComments() }, prefix + "Cannot reach Bugsink:", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLogs(t)
			tb := newTriageBugsink(t)
			c.setup(tb)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/mute", "")
			if r.status != 200 {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(r.body), &body); err != nil {
				t.Fatal(err)
			}
			warning, _ := body["warning"].(string)
			if strings.HasSuffix(c.warning, ":") {
				if !strings.HasPrefix(warning, c.warning+" ") {
					t.Fatalf("warning %q, want prefix %q", warning, c.warning)
				}
			} else if warning != c.warning {
				t.Fatalf("warning %q, want %q", warning, c.warning)
			}
			if body["status"] != "muted" || body["friendlyId"] != "SAMPLE-PYTHON-7" {
				t.Fatalf("body %s", r.body)
			}
			if len(tb.actionPosts()) != 1 || len(tb.commentPosts()) != 1 {
				t.Fatalf("Bugsink POSTs %+v", tb.posts())
			}
			logs.assertTriageLine(t, map[string]any{
				"action": "mute", "issue": "SAMPLE-PYTHON-7", "user": "ed", "role": "Editor", "status": 200, "comment": c.comment,
			})
			assertNoStore(t, r)
			assertReplyHasNoToken(t, r)
			logs.assertNoToken(t)
		})
	}
	t.Run("no warning on success", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/mute", "")
		if r.status != 200 || strings.Contains(r.body, "warning") || len(tb.commentPosts()) != 1 {
			t.Fatalf("status %d body %s posts %+v", r.status, r.body, tb.posts())
		}
		assertReplyHasNoToken(t, r)
	})
}

func TestTriageNoStore(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range triageActions {
		path, body := "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action)
		t.Run("200/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", path, body)
			if r.status != 200 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
		t.Run("200 with a warning/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			tb.answerComment(500, "")
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", path, body)
			if r.status != 200 || !strings.Contains(r.body, "warning") {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			assertNoStore(t, r)
		})
		t.Run("403 caller/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), nil, "POST", path, body)
			if r.status != 403 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
		t.Run("403 off/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, `{}`), editor, "POST", path, body)
			if r.status != 403 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
		t.Run("400 settings/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, `{"triageEnabled":"yes"}`), editor, "POST", path, body)
			if r.status != 400 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
		t.Run("400 id/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/a.b/"+action, body)
			if r.status != 400 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
		t.Run("502/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			tb.answer(400, `["Issue is already resolved."]`)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", path, body)
			if r.status != 502 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
	}
	for _, action := range []string{"mute-for", "mute-until"} {
		t.Run("400 body/"+action, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/"+action, `{}`)
			if r.status != 400 {
				t.Fatalf("status %d", r.status)
			}
			assertNoStore(t, r)
		})
	}
}

func TestTriageClearsIssueCache(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, action := range triageActions {
		for _, c := range []struct {
			name    string
			status  int
			body    string
			comment int
			reply   int
		}{
			{"Bugsink answers 200", 0, "", 0, 200},
			{"Bugsink answers 200, the comment fails", 0, "", 500, 200},
			{"Bugsink answers 400", 400, `["Issue is already resolved."]`, 0, 502},
		} {
			t.Run(c.name+"/"+action, func(t *testing.T) {
				tb := newTriageBugsink(t)
				tb.fake.pages = []string{inRange("SAMPLE-PYTHON-7")}
				tb.answer(c.status, c.body)
				if c.comment != 0 {
					tb.answerComment(c.comment, "")
				}
				ds := tb.datasource(t, triageOn)
				model := `{"queryType":"issues"}`
				frameOf(t, runQuery(t, ds, model))
				frameOf(t, runQuery(t, ds, model))
				if n := len(tb.fake.issueRequests()); n != 1 {
					t.Fatalf("%d list requests before the triage call, want 1 (cached)", n)
				}
				if r := callAs(t, ds, editor, "POST", "issues/SAMPLE-PYTHON-7/"+action, bodyFor(action)); r.status != c.reply {
					t.Fatalf("triage status %d body %s", r.status, r.body)
				}
				frameOf(t, runQuery(t, ds, model))
				if n := len(tb.fake.issueRequests()); n != 2 {
					t.Fatalf("%d list requests after the triage call, want 2", n)
				}
			})
		}
	}
}

func TestTriageDropsListReadInFlight(t *testing.T) {
	tb := newTriageBugsink(t)
	tb.fake.pages = []string{inRange("SAMPLE-PYTHON-7")}
	ds := tb.datasource(t, triageOn)
	model := `{"queryType":"issues"}`
	held, release := tb.holdNextList()
	done := make(chan *backend.QueryDataResponse)
	go func() {
		resp, _ := ds.QueryData(context.Background(), &backend.QueryDataRequest{Queries: []backend.DataQuery{
			{RefID: "A", JSON: json.RawMessage(model), TimeRange: backend.TimeRange{From: rangeFrom, To: rangeTo}},
		}})
		done <- resp
	}()
	<-held
	if r := callAs(t, ds, &backend.User{Login: "ed", Role: "Editor"}, "POST", "issues/SAMPLE-PYTHON-7/resolve", ""); r.status != 200 {
		t.Fatalf("triage status %d body %s", r.status, r.body)
	}
	close(release)
	if resp := <-done; resp == nil {
		t.Fatal("the query in flight returned no response")
	} else {
		frameOf(t, resp.Responses["A"])
	}
	frameOf(t, runQuery(t, ds, model))
	if n := len(tb.fake.issueRequests()); n != 2 {
		t.Fatalf("%d list requests, want 2: the list read before the triage call was cached", n)
	}
}
