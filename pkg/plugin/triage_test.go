// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

var triageActions = []string{"resolve", "reopen", "mute", "unmute"}

const triageOn = `{"browserUrl":"http://b","triageEnabled":true}`

// triageBugsink serves fakeBugsink's reads, answers every POST, and records every request.
type triageBugsink struct {
	fake    *fakeBugsink
	rec     *recordingServer
	mu      sync.Mutex
	status  int           // status of every POST, 0 for 200 with the issue in the state the action leads to
	body    string        // body of every POST when status is set
	held    chan struct{} // when set, the next issue list read reports here and waits for release
	release chan struct{}
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

// issueAfter is the issue Bugsink answers a successful triage POST to path with.
func issueAfter(path string) string {
	issue := issueAt("SAMPLE-PYTHON-7", rangeTo)
	switch {
	case strings.HasSuffix(path, "/resolve/"):
		return strings.Replace(issue, `"is_resolved":false`, `"is_resolved":true`, 1)
	case strings.HasSuffix(path, "/mute/"):
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

func (tb *triageBugsink) assertNoRequests(t *testing.T) {
	t.Helper()
	if seen := tb.rec.requests(); len(seen) != 0 {
		t.Fatalf("Bugsink saw %d requests: %+v", len(seen), seen)
	}
}

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
	wantStatus := map[string]string{"resolve": "resolved", "reopen": "open", "mute": "muted", "unmute": "open"}
	for _, user := range []*backend.User{{Login: "ed", Role: "Editor"}, {Login: "ad", Role: "Admin"}} {
		for _, action := range triageActions {
			t.Run(user.Role+" "+action, func(t *testing.T) {
				logs := captureLogs(t)
				tb := newTriageBugsink(t)
				r := callAs(t, tb.datasource(t, triageOn), user, "POST", "issues/SAMPLE-PYTHON-7/"+action, "")
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
				posts := tb.posts()
				if len(posts) != 1 || posts[0].path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/"+action+"/" {
					t.Fatalf("Bugsink POSTs %+v", posts)
				}
				want := map[string]any{"action": action, "issue": "SAMPLE-PYTHON-7", "user": user.Login, "status": 200}
				if lines := logs.triageLines(t); len(lines) != 1 || !reflect.DeepEqual(lines[0], want) {
					t.Fatalf("triage log lines %v, want one %v", lines, want)
				}
				assertReplyHasNoToken(t, r)
				logs.assertNoToken(t)
			})
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
					r := callAs(t, tb.datasource(t, s.jsonData), u.user, "POST", "issues/SAMPLE-PYTHON-7/"+action, "")
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
	t.Run("refused before the id is read", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageOn), &backend.User{Role: "Viewer"}, "POST", "issues/a.b/resolve", "")
		if r.status != 403 || messageOf(t, r) != msgCallerRefused {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		tb.assertNoRequests(t)
	})
}

func TestTriageMethods(t *testing.T) {
	users := []*backend.User{{Role: "Admin"}, {Role: "Viewer"}, nil}
	for _, user := range users {
		for _, method := range []string{"GET", "PUT", "PATCH", "DELETE", "HEAD"} {
			for _, action := range triageActions {
				t.Run(fmt.Sprintf("%v/%s/%s", user, method, action), func(t *testing.T) {
					tb := newTriageBugsink(t)
					r := callAs(t, tb.datasource(t, triageOn), user, method, "issues/SAMPLE-PYTHON-7/"+action, "")
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
	for _, c := range []struct{ name, jsonData, path string }{
		{"switch absent", `{"browserUrl":"http://b"}`, "issues/SAMPLE-PYTHON-7/resolve"},
		{"switch off", `{"browserUrl":"http://b","triageEnabled":false}`, "issues/SAMPLE-PYTHON-7/mute"},
		{"no json data", "", "issues/SAMPLE-PYTHON-7/unmute"},
		{"off before the id is read", `{"triageEnabled":false}`, "issues/a.b/reopen"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, c.jsonData), editor, "POST", c.path, "")
			if r.status != 403 || messageOf(t, r) != msgTriageOff {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
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
					r := callWithHeaders(t, tb.datasource(t, triageOn), user, "POST", "issues/SAMPLE-PYTHON-7/"+action, "", h.headers)
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
	for _, c := range []struct {
		name, jsonData, path string
		user                 *backend.User
		status               int
		message              string
	}{
		{"caller refused first", triageOn, "issues/SAMPLE-PYTHON-7/resolve", &backend.User{Role: "Viewer"}, 403, msgCallerRefused},
		{"switch checked first", `{"browserUrl":"http://b"}`, "issues/SAMPLE-PYTHON-7/resolve", editor, 403, msgTriageOff},
		{"settings checked first", `{"triageEnabled":"yes"}`, "issues/SAMPLE-PYTHON-7/resolve", editor, 400, msgSettingsInvalid},
		{"refused before the id is read", triageOn, "issues/a.b/resolve", editor, 403, msgTriageHeader},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callWithHeaders(t, tb.datasource(t, c.jsonData), c.user, "POST", c.path, "", nil)
			if r.status != c.status || messageOf(t, r) != c.message {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
	}
}

func TestTriageSettingsInvalid(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, c := range []struct{ name, jsonData, path, message string }{
		{"switch not a boolean", `{"triageEnabled":"yes"}`, "issues/SAMPLE-PYTHON-7/resolve", msgSettingsInvalid},
		{"browser url", `{"triageEnabled":true,"browserUrl":"ftp://x"}`, "issues/SAMPLE-PYTHON-7/mute", msgBrowserURLScheme},
		{"invalid before the id is read", `{"triageEnabled":"yes"}`, "issues/a.b/reopen", msgSettingsInvalid},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, c.jsonData), editor, "POST", c.path, "")
			if r.status != 400 || messageOf(t, r) != c.message {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
	}
	t.Run("no url", func(t *testing.T) {
		r := callAs(t, newInstance(t, "", secretToken, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 400 || messageOf(t, r) != msgEnterURL {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
	})
}

func TestTriageRejectsIDs(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	long := strings.Repeat("a", 101)
	for _, c := range []struct{ path, id string }{
		{"issues/../resolve", ".."},
		{"issues/./resolve", "."},
		{"issues/%2e%2e/reopen", ".."},
		{"issues/a%2Fb/resolve", "a/b"},
		{"issues/x%20y/mute", "x y"},
		{"issues/x%3Fy/unmute", "x?y"},
		{"issues//reopen", ""},
		{"issues/a.b/resolve", "a.b"},
		{"issues/" + long + "/resolve", long},
	} {
		t.Run(c.path, func(t *testing.T) {
			tb := newTriageBugsink(t)
			r := callAs(t, tb.datasource(t, triageOn), editor, "POST", c.path, "")
			if r.status != 400 || messageOf(t, r) != notAnIssueID(c.id) {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
			tb.assertNoRequests(t)
		})
	}
	t.Run("100 characters is an id", func(t *testing.T) {
		tb := newTriageBugsink(t)
		id := strings.Repeat("a", 100)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/"+id+"/resolve", "")
		if r.status != 200 || len(tb.posts()) != 1 {
			t.Fatalf("status %d body %s posts %+v", r.status, r.body, tb.posts())
		}
	})
}

func TestTriageSendsOnlyTheAction(t *testing.T) {
	logs := captureLogs(t)
	tb := newTriageBugsink(t)
	editor := &backend.User{Login: "ed", Role: "Editor"}
	r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/mute?gte_threshold=5",
		`{"period_name":"day","nr_of_periods":2}`)
	if r.status != 200 {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	posts := tb.posts()
	if len(posts) != 1 {
		t.Fatalf("Bugsink POSTs %+v", posts)
	}
	if p := posts[0]; p.path != "/api/canonical/0/issues/SAMPLE-PYTHON-7/mute/" || p.body != "" || p.rawQuery != "" {
		t.Fatalf("Bugsink saw %+v", p)
	}
	for _, l := range logs.all() {
		if s := l.String(); strings.Contains(s, "period_name") || strings.Contains(s, "gte_threshold") {
			t.Fatalf("log line holds the request: %s", s)
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
			want := map[string]any{"action": "resolve", "issue": "SAMPLE-PYTHON-7", "user": "ed", "status": c.status}
			if lines := logs.triageLines(t); len(lines) != 1 || !reflect.DeepEqual(lines[0], want) {
				t.Fatalf("triage log lines %v, want one %v", lines, want)
			}
			assertNoStore(t, r)
			assertReplyHasNoToken(t, r)
			logs.assertNoToken(t)
		})
	}

	t.Run("closed server", func(t *testing.T) {
		logs := captureLogs(t)
		ds := newInstance(t, closedPortURL(t), secretToken, triageOn)
		r := callAs(t, ds, editor, "POST", "issues/SAMPLE-PYTHON-7/unmute", "")
		if r.status != 502 || !strings.HasPrefix(messageOf(t, r), "Cannot reach Bugsink: ") {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		want := map[string]any{"action": "unmute", "issue": "SAMPLE-PYTHON-7", "user": "ed", "status": 0}
		if lines := logs.triageLines(t); len(lines) != 1 || !reflect.DeepEqual(lines[0], want) {
			t.Fatalf("triage log lines %v, want one %v", lines, want)
		}
		assertNoStore(t, r)
		assertReplyHasNoToken(t, r)
		logs.assertNoToken(t)
	})
}

func TestTriageNoStore(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	t.Run("200", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 200 {
			t.Fatalf("status %d", r.status)
		}
		assertNoStore(t, r)
	})
	t.Run("403 caller", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageOn), nil, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 403 {
			t.Fatalf("status %d", r.status)
		}
		assertNoStore(t, r)
	})
	t.Run("403 off", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, `{}`), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 403 {
			t.Fatalf("status %d", r.status)
		}
		assertNoStore(t, r)
	})
	t.Run("400 settings", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, `{"triageEnabled":"yes"}`), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 400 {
			t.Fatalf("status %d", r.status)
		}
		assertNoStore(t, r)
	})
	t.Run("400 id", func(t *testing.T) {
		tb := newTriageBugsink(t)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/a.b/resolve", "")
		if r.status != 400 {
			t.Fatalf("status %d", r.status)
		}
		assertNoStore(t, r)
	})
	t.Run("502", func(t *testing.T) {
		tb := newTriageBugsink(t)
		tb.answer(400, `["Issue is already resolved."]`)
		r := callAs(t, tb.datasource(t, triageOn), editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", "")
		if r.status != 502 {
			t.Fatalf("status %d", r.status)
		}
		assertNoStore(t, r)
	})
}

func TestTriageClearsIssueCache(t *testing.T) {
	editor := &backend.User{Login: "ed", Role: "Editor"}
	for _, c := range []struct {
		name   string
		status int
		body   string
		reply  int
	}{
		{"Bugsink answers 200", 0, "", 200},
		{"Bugsink answers 400", 400, `["Issue is already resolved."]`, 502},
	} {
		t.Run(c.name, func(t *testing.T) {
			tb := newTriageBugsink(t)
			tb.fake.pages = []string{inRange("SAMPLE-PYTHON-7")}
			tb.answer(c.status, c.body)
			ds := tb.datasource(t, triageOn)
			model := `{"queryType":"issues"}`
			frameOf(t, runQuery(t, ds, model))
			frameOf(t, runQuery(t, ds, model))
			if n := len(tb.fake.issueRequests()); n != 1 {
				t.Fatalf("%d list requests before the triage call, want 1 (cached)", n)
			}
			if r := callAs(t, ds, editor, "POST", "issues/SAMPLE-PYTHON-7/resolve", ""); r.status != c.reply {
				t.Fatalf("triage status %d body %s", r.status, r.body)
			}
			frameOf(t, runQuery(t, ds, model))
			if n := len(tb.fake.issueRequests()); n != 2 {
				t.Fatalf("%d list requests after the triage call, want 2", n)
			}
		})
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
