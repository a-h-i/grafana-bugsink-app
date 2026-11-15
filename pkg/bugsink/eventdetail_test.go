// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func fixtureBytes(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "events", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// getFixture serves a fixture through a test server and reads it with GetEvent.
func getFixture(t *testing.T, name string) EventDetail {
	t.Helper()
	body := fixtureBytes(t, name)
	var head struct{ ID string }
	if err := json.Unmarshal(body, &head); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
	ev, err := c.GetEvent(context.Background(), uuid.MustParse(head.ID))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestGetEventRequest(t *testing.T) {
	var path, auth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, eventJSON(eventUUID, sdkUUID))
	})
	ev, err := c.GetEvent(context.Background(), uuid.MustParse(eventUUID))
	if err != nil {
		t.Fatal(err)
	}
	want := Event{
		ID: eventUUID, EventID: sdkUUID, IssueID: issueUUID, ProjectID: 1,
		Timestamp:  time.Date(2026, 11, 15, 4, 47, 36, 0, time.UTC),
		IngestedAt: time.Date(2026, 11, 15, 4, 47, 37, 0, time.UTC),
	}
	if path != "/api/canonical/0/events/"+eventUUID+"/" || auth != "Bearer tok" || !reflect.DeepEqual(ev.Event, want) {
		t.Fatalf("path %q auth %q event %+v", path, auth, ev.Event)
	}

	c = newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"detail":"No Event matches the given query."}`)
	})
	_, err = c.GetEvent(context.Background(), uuid.MustParse(eventUUID))
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Detail != "No Event matches the given query." {
		t.Fatalf("error = %v", err)
	}
}

func TestParsePythonUnhandled(t *testing.T) {
	ev := getFixture(t, "python-unhandled")
	if len(ev.Exceptions) != 1 {
		t.Fatalf("exceptions = %d", len(ev.Exceptions))
	}
	ex := ev.Exceptions[0]
	if len(ex.Frames) != 26 || ex.FramesOmitted != 0 {
		t.Fatalf("frames = %d omitted %d", len(ex.Frames), ex.FramesOmitted)
	}
	for i, f := range ex.Frames {
		if f.InApp != (i >= 23) {
			t.Fatalf("frame %d InApp = %v", i, f.InApp)
		}
	}
	last := ex.Frames[25]
	if last.Filename != "app.py" || last.Function != "_fail" || last.Lineno == nil || *last.Lineno != 60 ||
		last.Colno != nil || last.ContextLine == nil ||
		*last.ContextLine != `    raise RuntimeError(_message("unhandled failure", variant))` ||
		len(last.PreContext) != 5 || len(last.PostContext) != 5 {
		t.Fatalf("last frame = %+v", last)
	}
	if ex.Mechanism != "starlette" || ex.Handled == nil || *ex.Handled {
		t.Fatalf("mechanism %q handled %v", ex.Mechanism, ex.Handled)
	}
	if ev.Environment != "development" || ev.Release != "sample-python@0.1.0" || ev.Platform != "python" ||
		ev.TraceID != "97dc67918c526f3d407fdcec08e956da" || ev.StacktraceMD != "" {
		t.Fatalf("event = %+v", ev)
	}
	if !reflect.DeepEqual(ev.Tags, []KeyValue{{"service", "sample-python"}}) {
		t.Fatalf("tags = %v", ev.Tags)
	}
	if len(ev.Contexts) != 2 || ev.Contexts[0].Name != "runtime" || ev.Contexts[1].Name != "trace" {
		t.Fatalf("contexts = %+v", ev.Contexts)
	}
	if len(ev.Breadcrumbs) != 2 || ev.Breadcrumbs[0].Timestamp == nil ||
		!ev.Breadcrumbs[0].Timestamp.Equal(time.Date(2026, 11, 13, 23, 29, 57, 353657000, time.UTC)) ||
		ev.Breadcrumbs[1].Category != "uvicorn.access" || ev.Breadcrumbs[0].Type != "log" ||
		ev.Breadcrumbs[0].Level != "info" || ev.Breadcrumbs[1].Timestamp == nil {
		t.Fatalf("breadcrumbs = %+v", ev.Breadcrumbs)
	}
}

func TestParseHandled(t *testing.T) {
	ex := getFixture(t, "python-handled").Exceptions
	if len(ex) != 1 || ex[0].Handled == nil || !*ex[0].Handled || ex[0].Mechanism != "generic" {
		t.Fatalf("exceptions = %+v", ex)
	}
}

func TestParseChained(t *testing.T) {
	ex := getFixture(t, "python-chained").Exceptions
	if len(ex) != 2 || ex[0].Type != "JSONDecodeError" || ex[0].Module != "json.decoder" ||
		ex[1].Type != "RuntimeError" || len(ex[0].Frames) != 4 || len(ex[1].Frames) != 1 {
		t.Fatalf("exceptions = %+v", ex)
	}
}

func TestParseMessageWithThreads(t *testing.T) {
	ev := getFixture(t, "python-message-threads")
	var raw struct {
		StacktraceMD string `json:"stacktrace_md"`
	}
	if err := json.Unmarshal(fixtureBytes(t, "python-message-threads"), &raw); err != nil {
		t.Fatal(err)
	}
	if len(ev.Exceptions) != 0 || ev.Message == "" || ev.StacktraceMD != raw.StacktraceMD || raw.StacktraceMD == "" {
		t.Fatalf("exceptions %d message %q md %q", len(ev.Exceptions), ev.Message, ev.StacktraceMD)
	}

	ev = getFixture(t, "message-only")
	if ev.Message != "probe event for project 2" || ev.StacktraceMD != "" || len(ev.Exceptions) != 0 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestParseJavaScript(t *testing.T) {
	ev := getFixture(t, "javascript-error")
	if len(ev.Exceptions) != 2 || ev.Exceptions[0].Type != "TypeError" || ev.Exceptions[0].Mechanism != "chained" ||
		ev.Exceptions[1].Handled == nil || *ev.Exceptions[1].Handled {
		t.Fatalf("exceptions = %+v", ev.Exceptions)
	}
	f := ev.Exceptions[0].Frames[0]
	if f.Filename != "http://localhost:8820/assets/index.js" || f.Function != "run" || f.Lineno == nil ||
		*f.Lineno != 10 || f.Colno == nil || *f.Colno != 5 || !f.InApp || f.ContextLine != nil {
		t.Fatalf("frame = %+v", f)
	}
	if len(ev.Breadcrumbs) != 1 || ev.Breadcrumbs[0].Timestamp == nil ||
		!ev.Breadcrumbs[0].Timestamp.Equal(time.Date(2025, 11, 15, 8, 0, 0, 250000000, time.UTC)) ||
		!reflect.DeepEqual(ev.Breadcrumbs[0].Data, []KeyValue{{"x", "1"}}) {
		t.Fatalf("breadcrumbs = %+v", ev.Breadcrumbs)
	}
	if !reflect.DeepEqual(ev.Tags, []KeyValue{{"browser", "Chrome 131"}, {"service", "sample-js"}}) {
		t.Fatalf("tags = %v", ev.Tags)
	}
	if ev.Message != "x" || ev.Platform != "javascript" || len(ev.Contexts) != 1 || ev.Contexts[0].Name != "browser" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestParseNodeHandled(t *testing.T) {
	ev := getFixture(t, "node-handled")
	if ev.Platform != "node" || ev.Environment != "development" || ev.Release != "sample-js@0.1.0" {
		t.Fatalf("event = %+v", ev)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(ev.TraceID) {
		t.Fatalf("trace id = %q", ev.TraceID)
	}
	if len(ev.Exceptions) != 2 || ev.Exceptions[0].Type != "TypeError" || ev.Exceptions[0].Value != "order total is not a number" ||
		ev.Exceptions[1].Type != "Error" {
		t.Fatalf("exceptions = %+v", ev.Exceptions)
	}
	var app *Frame
	for _, f := range ev.Exceptions[0].Frames {
		if f.InApp && strings.HasSuffix(f.Filename, "app.mjs") {
			app = &f
			break
		}
	}
	if app == nil || app.Lineno == nil || app.Colno == nil || app.ContextLine == nil {
		t.Fatalf("in-app frame = %+v", app)
	}
	if !reflect.DeepEqual(ev.Tags, []KeyValue{{"service", "sample-js"}}) {
		t.Fatalf("tags = %v", ev.Tags)
	}
	if len(ev.Breadcrumbs) != 1 || ev.Breadcrumbs[0].Message != "about to fail" || ev.Breadcrumbs[0].Timestamp == nil {
		t.Fatalf("breadcrumbs = %+v", ev.Breadcrumbs)
	}
}

func TestFixturesAreScrubbed(t *testing.T) {
	forbidden := regexp.MustCompile(`server_name|\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b|/tmp/|/home/|0x[0-9a-fA-F]{8}|Mozilla/`)
	files, err := filepath.Glob(filepath.Join("testdata", "events", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("fixtures %v, err %v", files, err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := forbidden.Find(b); m != nil {
			t.Errorf("%s contains %q", f, m)
		}
	}
}

func payload(t testing.TB, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type obj = map[string]any

func exceptionOf(name string, frames int) obj {
	fs := make([]obj, frames)
	for i := range fs {
		fs[i] = obj{"filename": fmt.Sprintf("f%d.py", i), "lineno": i}
	}
	return obj{"type": name, "stacktrace": obj{"frames": fs}}
}

func TestParseBounds(t *testing.T) {
	t.Run("exceptions keep the last ten", func(t *testing.T) {
		var vs []obj
		for i := 0; i < 12; i++ {
			vs = append(vs, exceptionOf(fmt.Sprintf("E%d", i), 1))
		}
		ev := parseEvent(payload(t, obj{"exception": obj{"values": vs}}), "")
		if len(ev.Exceptions) != MaxExceptions || ev.Exceptions[0].Type != "E2" || ev.Exceptions[9].Type != "E11" || !ev.ExceptionsCut {
			t.Fatalf("exceptions %d cut %v", len(ev.Exceptions), ev.ExceptionsCut)
		}
	})
	t.Run("frames keep the last 250", func(t *testing.T) {
		ev := parseEvent(payload(t, obj{"exception": obj{"values": []obj{exceptionOf("E", 300)}}}), "")
		ex := ev.Exceptions[0]
		if len(ex.Frames) != MaxFrames || ex.FramesOmitted != 50 || ex.Frames[0].Filename != "f50.py" ||
			ex.Frames[249].Filename != "f299.py" || ev.ExceptionsCut {
			t.Fatalf("frames %d omitted %d", len(ex.Frames), ex.FramesOmitted)
		}
	})
	t.Run("context lines", func(t *testing.T) {
		lines := []string{"1", "2", "3", "4", "5", "6", "7", "8"}
		frame := obj{"filename": "a", "pre_context": lines, "post_context": lines, "context_line": "x"}
		ev := parseEvent(payload(t, obj{"exception": []obj{{"stacktrace": obj{"frames": []obj{frame}}}}}), "")
		f := ev.Exceptions[0].Frames[0]
		if !reflect.DeepEqual(f.PreContext, lines[3:]) || !reflect.DeepEqual(f.PostContext, lines[:5]) {
			t.Fatalf("pre %v post %v", f.PreContext, f.PostContext)
		}
	})
	t.Run("breadcrumbs keep the newest 100", func(t *testing.T) {
		var bs []obj
		for i := 0; i < 150; i++ {
			bs = append(bs, obj{"message": fmt.Sprint(i)})
		}
		ev := parseEvent(payload(t, obj{"breadcrumbs": obj{"values": bs}}), "")
		if len(ev.Breadcrumbs) != MaxBreadcrumbs || ev.Breadcrumbs[0].Message != "50" ||
			ev.Breadcrumbs[99].Message != "149" || !ev.BreadcrumbsCut {
			t.Fatalf("breadcrumbs %d cut %v", len(ev.Breadcrumbs), ev.BreadcrumbsCut)
		}
	})
	t.Run("long values are cut by characters", func(t *testing.T) {
		long := strings.Repeat("é", 200_000)
		frame := obj{"filename": "min.js", "context_line": long}
		ev := parseEvent(payload(t, obj{
			"exception": []obj{{"stacktrace": obj{"frames": []obj{frame}}}},
			"tags":      obj{"k": strings.Repeat("x", 5000)},
		}), "")
		want := strings.Repeat("é", MaxValueLength) + "…"
		if got := *ev.Exceptions[0].Frames[0].ContextLine; got != want || !ev.ValuesCut {
			t.Fatalf("context line length %d cut %v", utf8.RuneCountInString(got), ev.ValuesCut)
		}
		if got := ev.Tags[0].Value; got != strings.Repeat("x", MaxValueLength)+"…" {
			t.Fatalf("tag length %d", len(got))
		}
	})
	t.Run("values at the limit are kept", func(t *testing.T) {
		ev := parseEvent(payload(t, obj{"tags": obj{"k": strings.Repeat("x", MaxValueLength)}}), "")
		if ev.ValuesCut || len(ev.Tags[0].Value) != MaxValueLength {
			t.Fatalf("cut %v length %d", ev.ValuesCut, len(ev.Tags[0].Value))
		}
	})
	t.Run("entries beyond the bound are dropped by key", func(t *testing.T) {
		tags, data := obj{}, obj{}
		for i := 0; i < 250; i++ {
			tags[fmt.Sprintf("k%03d", i)] = "v"
			data[fmt.Sprintf("d%03d", i)] = "v"
		}
		tags["k249"] = strings.Repeat("x", 5000)
		ev := parseEvent(payload(t, obj{"tags": tags, "breadcrumbs": []obj{{"data": data}}}), "")
		if len(ev.Tags) != MaxEntries || ev.Tags[0].Key != "k000" || ev.Tags[199].Key != "k199" || !ev.EntriesCut {
			t.Fatalf("tags %d cut %v", len(ev.Tags), ev.EntriesCut)
		}
		if len(ev.Breadcrumbs[0].Data) != MaxEntries || ev.ValuesCut {
			t.Fatalf("data %d valuesCut %v", len(ev.Breadcrumbs[0].Data), ev.ValuesCut)
		}
	})
	t.Run("contexts beyond the bound are dropped by name", func(t *testing.T) {
		cs := obj{}
		for i := 0; i < 250; i++ {
			cs[fmt.Sprintf("c%03d", i)] = obj{"a": 1}
		}
		ev := parseEvent(payload(t, obj{"contexts": cs}), "")
		if len(ev.Contexts) != MaxEntries || ev.Contexts[199].Name != "c199" || !ev.EntriesCut {
			t.Fatalf("contexts %d cut %v", len(ev.Contexts), ev.EntriesCut)
		}
	})
	t.Run("a long line among the dropped context lines sets no cut", func(t *testing.T) {
		pre := []string{strings.Repeat("x", 2000), "1", "2", "3", "4", "5"}
		frame := obj{"filename": "a", "pre_context": pre, "post_context": []string{"1", "2", "3", "4", "5", strings.Repeat("y", 2000)}}
		ev := parseEvent(payload(t, obj{"exception": []obj{{"stacktrace": obj{"frames": []obj{frame}}}}}), "")
		if ev.ValuesCut || len(ev.Exceptions[0].Frames[0].PreContext) != 5 || len(ev.Exceptions[0].Frames[0].PostContext) != 5 {
			t.Fatalf("cut %v frame %+v", ev.ValuesCut, ev.Exceptions[0].Frames[0])
		}
	})
	t.Run("stack trace text is cut on a character boundary", func(t *testing.T) {
		md := "a" + strings.Repeat("é", 75_000)
		thread := obj{"threads": obj{"values": []obj{{"stacktrace": obj{"frames": []obj{{"filename": "a"}}}}}}}
		ev := parseEvent(payload(t, thread), md)
		if len(ev.StacktraceMD) != MaxStacktraceMD-1 || !ev.StacktraceMDCut || !utf8.ValidString(ev.StacktraceMD) {
			t.Fatalf("length %d cut %v valid %v", len(ev.StacktraceMD), ev.StacktraceMDCut, utf8.ValidString(ev.StacktraceMD))
		}
		ev = parseEvent(payload(t, thread), "short")
		if ev.StacktraceMD != "short" || ev.StacktraceMDCut {
			t.Fatalf("md %q cut %v", ev.StacktraceMD, ev.StacktraceMDCut)
		}
	})
}

func TestParseStacktraceFallback(t *testing.T) {
	frames := []obj{{"filename": "a"}}
	cases := []struct {
		name string
		data obj
		want string
	}{
		{"event-level stacktrace", obj{"stacktrace": obj{"frames": frames}}, "md"},
		{"thread list", obj{"threads": []obj{{"stacktrace": obj{"frames": frames}}}}, "md"},
		{"exception frames win", obj{"exception": []obj{{"stacktrace": obj{"frames": frames}}}, "threads": obj{"values": []obj{{"stacktrace": obj{"frames": frames}}}}}, ""},
		{"exception without frames", obj{"exception": []obj{{"type": "E"}}, "stacktrace": obj{"frames": frames}}, "md"},
		{"no frames anywhere", obj{"message": "m"}, ""},
		{"empty thread frames", obj{"threads": obj{"values": []obj{{"stacktrace": obj{"frames": []obj{}}}}}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseEvent(payload(t, c.data), "md").StacktraceMD; got != c.want {
				t.Fatalf("StacktraceMD = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseMessage(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"logentry formatted", `{"logentry":{"formatted":"f","message":"m"},"message":"p"}`, "f"},
		{"logentry message", `{"logentry":{"message":"m"},"message":"p"}`, "m"},
		{"plain message", `{"message":"p"}`, "p"},
		{"message object", `{"message":{"formatted":"mf"}}`, "mf"},
		{"message object message", `{"message":{"message":"mm"}}`, "mm"},
		{"wrong types", `{"logentry":5,"message":7}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseEvent(json.RawMessage(c.data), "").Message; got != c.want {
				t.Fatalf("Message = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseTolerance(t *testing.T) {
	for _, data := range []string{`null`, `[]`, `"x"`, `5`, `true`, ``, `{`, `{}`} {
		ev := parseEvent(json.RawMessage(data), "")
		if len(ev.Exceptions) != 0 || len(ev.Tags) != 0 || ev.Message != "" {
			t.Fatalf("data %q gave %+v", data, ev)
		}
	}

	ev := parseEvent(json.RawMessage(`{
		"level":"error","tags":5,"exception":{"values":"x"},"contexts":{"trace":3,"ok":{"a":1}},
		"environment":{"x":1},"release":"r1","breadcrumbs":"no"}`), "")
	if ev.Level != "error" || ev.Release != "r1" || ev.Environment != "" || len(ev.Tags) != 0 ||
		len(ev.Exceptions) != 0 || len(ev.Breadcrumbs) != 0 ||
		!reflect.DeepEqual(ev.Contexts, []EventContext{{Name: "ok", Fields: []KeyValue{{"a", "1"}}}}) {
		t.Fatalf("event = %+v", ev)
	}

	ev = parseEvent(json.RawMessage(`{
		"tags":{"a":{"a":1},"b":null,"c":2.5,"d":true},
		"exception":[{"type":"E","value":{"k":1},"mechanism":"m","stacktrace":{"frames":[
			{"filename":"f","lineno":"7","colno":1.5,"in_app":"yes","pre_context":[1,"x",null],"context_line":9},
			"junk", null, 3, {"abs_path":"/p","in_app":true}]}}]}`), "")
	if !reflect.DeepEqual(ev.Tags, []KeyValue{{"a", `{"a":1}`}, {"b", ""}, {"c", "2.5"}, {"d", "true"}}) {
		t.Fatalf("tags = %v", ev.Tags)
	}
	ex := ev.Exceptions[0]
	if ex.Type != "E" || ex.Value != `{"k":1}` || ex.Mechanism != "" || ex.Handled != nil || len(ex.Frames) != 2 {
		t.Fatalf("exception = %+v", ex)
	}
	f := ex.Frames[0]
	if f.Filename != "f" || f.Lineno != nil || f.Colno != nil || f.InApp || f.ContextLine != nil ||
		!reflect.DeepEqual(f.PreContext, []string{"", "x", ""}) {
		t.Fatalf("frame = %+v", f)
	}
	if g := ex.Frames[1]; g.Filename != "/p" || !g.InApp {
		t.Fatalf("frame = %+v", g)
	}
}

func TestParseBreadcrumbTimestamps(t *testing.T) {
	bs := parseEvent(json.RawMessage(`{"breadcrumbs":[
		{"timestamp":"2025-11-15T08:00:00Z"},{"timestamp":1763193600},{"timestamp":"yesterday"},
		{"timestamp":1e30},{"timestamp":-1e30},{"timestamp":null},{}]}`), "").Breadcrumbs
	if len(bs) != 7 {
		t.Fatalf("breadcrumbs = %+v", bs)
	}
	want := time.Date(2025, 11, 15, 8, 0, 0, 0, time.UTC)
	if bs[0].Timestamp == nil || !bs[0].Timestamp.Equal(want) || bs[1].Timestamp == nil || !bs[1].Timestamp.Equal(want) {
		t.Fatalf("times = %v %v", bs[0].Timestamp, bs[1].Timestamp)
	}
	for _, b := range bs[2:] {
		if b.Timestamp != nil {
			t.Fatalf("timestamp = %v", b.Timestamp)
		}
	}
}

func TestParseBreadcrumbTimeWithoutZone(t *testing.T) {
	bs := parseEvent(json.RawMessage(`{"breadcrumbs":[{"timestamp":"2026-11-15T07:52:59.387"},{"timestamp":"2026-11-15T07:52:59+02:00"}]}`), "").Breadcrumbs
	if len(bs) != 2 || bs[0].Timestamp == nil || bs[1].Timestamp == nil {
		t.Fatalf("breadcrumbs = %+v", bs)
	}
	if got := bs[0].Timestamp.UTC().Format(time.RFC3339Nano); got != "2026-11-15T07:52:59.387Z" {
		t.Fatalf("time = %s", got)
	}
	if got := bs[1].Timestamp.UTC().Format(time.RFC3339); got != "2026-11-15T05:52:59Z" {
		t.Fatalf("offset time = %s", got)
	}
}

// wrongValues replaces one node of a payload at a time.
var wrongValues = []any{nil, 0, -1, 1e300, "", "x", true, []any{}, []any{nil, 1, "a"}, obj{}, obj{"values": nil}, obj{"values": "x"}, obj{"frames": 5}}

func mutations(v any, emit func()) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			for _, w := range wrongValues {
				x[k] = w
				emit()
			}
			x[k] = child
			mutations(child, emit)
		}
	case []any:
		for i, child := range x {
			for _, w := range wrongValues {
				x[i] = w
				emit()
			}
			x[i] = child
			mutations(child, emit)
		}
	}
}

func TestParseNeverPanicsOnMutatedFixtures(t *testing.T) {
	for _, name := range []string{"python-handled", "python-chained", "python-message-threads", "message-only", "javascript-error", "node-handled"} {
		var doc struct {
			Data         any    `json:"data"`
			StacktraceMD string `json:"stacktrace_md"`
		}
		if err := json.Unmarshal(fixtureBytes(t, name), &doc); err != nil {
			t.Fatal(err)
		}
		n := 0
		mutations(doc.Data, func() {
			n++
			parseEvent(payload(t, doc.Data), doc.StacktraceMD)
		})
		if n < 20 {
			t.Fatalf("%s: only %d mutations", name, n)
		}
	}
}

func TestParseDeepNesting(t *testing.T) {
	deep := strings.Repeat("[", 20_000) + strings.Repeat("]", 20_000)
	for _, data := range []string{
		`{"tags":` + deep + `}`, `{"exception":` + deep + `}`, `{"contexts":{"a":` + deep + `}}`,
		`{"tags":{"k":` + deep + `}}`, `{"breadcrumbs":[{"data":{"k":` + deep + `}}]}`,
		strings.Repeat(`{"a":`, 20_000) + `1` + strings.Repeat(`}`, 20_000),
	} {
		parseEvent(json.RawMessage(data), "")
	}
}

func FuzzParseEvent(f *testing.F) {
	for _, name := range []string{"python-unhandled", "python-chained", "python-message-threads", "message-only", "javascript-error", "node-handled"} {
		var doc struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(fixtureBytes(f, name), &doc); err != nil {
			f.Fatal(err)
		}
		f.Add([]byte(doc.Data))
	}
	f.Add([]byte(`{"exception":{"values":[{"stacktrace":{"frames":[{"lineno":1e999}]}}]}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		ev := parseEvent(json.RawMessage(data), "md")
		if len(ev.Exceptions) > MaxExceptions || len(ev.Breadcrumbs) > MaxBreadcrumbs {
			t.Fatalf("unbounded result: %d exceptions, %d breadcrumbs", len(ev.Exceptions), len(ev.Breadcrumbs))
		}
	})
}
