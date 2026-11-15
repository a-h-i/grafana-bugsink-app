// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type resourceReply struct {
	status  int
	body    string
	headers map[string][]string
}

type collectingSender struct {
	reply *resourceReply
	allow string
}

func (c *collectingSender) Send(r *backend.CallResourceResponse) error {
	c.reply = &resourceReply{status: r.Status, body: string(r.Body), headers: r.Headers}
	if v := r.Headers["Allow"]; len(v) > 0 {
		c.allow = v[0]
	}
	return nil
}

func callResource(t *testing.T, ds *Datasource, method, path string) resourceReply {
	t.Helper()
	sender := &collectingSender{}
	err := ds.CallResource(context.Background(), &backend.CallResourceRequest{Method: method, Path: path}, sender)
	if err != nil {
		t.Fatalf("CallResource returned error: %v", err)
	}
	if sender.reply == nil {
		t.Fatal("no response sent")
	}
	return *sender.reply
}

func messageOf(t *testing.T, r resourceReply) string {
	t.Helper()
	var m struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(r.body), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", r.body, err)
	}
	return m.Message
}

func TestProjectsResourceList(t *testing.T) {
	f := newFakeBugsink(t)
	f.projectsBody = `{"id":2,"name":"sample-js","extra":1},{"id":1,"name":"sample-python"}`
	r := callResource(t, f.datasource(t), "GET", "projects")
	want := `[{"id":2,"name":"sample-js"},{"id":1,"name":"sample-python"}]`
	if r.status != 200 || strings.TrimSpace(r.body) != want {
		t.Fatalf("status %d body %s, want %s", r.status, r.body, want)
	}
}

func TestProjectsResourceEmptyList(t *testing.T) {
	f := newFakeBugsink(t)
	f.projectsBody = " "
	r := callResource(t, f.datasource(t), "GET", "projects")
	if r.status != 200 || strings.TrimSpace(r.body) != "[]" {
		t.Fatalf("status %d body %s, want []", r.status, r.body)
	}
}

func TestProjectsResourceErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{"unauthorized", 401, `{"detail":"Invalid token."}`, "Bugsink rejected the API token."},
		{"capability", 403, `{"detail":"This token does not have the required capability: projects:read."}`,
			"The API token lacks projects:read."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.projectsStatus, f.errorBody = c.status, c.body
			r := callResource(t, f.datasource(t), "GET", "projects")
			if r.status != 502 || messageOf(t, r) != c.message || strings.Contains(r.body, secretToken) {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
		})
	}

	t.Run("closed server", func(t *testing.T) {
		r := callResource(t, newInstance(t, closedPortURL(t), secretToken, ""), "GET", "projects")
		if r.status != 502 || !strings.HasPrefix(messageOf(t, r), "Cannot reach Bugsink: ") || strings.Contains(r.body, secretToken) {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
	})
}

func TestProjectsResourceSettingsError(t *testing.T) {
	r := callResource(t, newInstance(t, "", secretToken, ""), "GET", "projects")
	if r.status != 400 || messageOf(t, r) != msgEnterURL {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
}

func TestResourceRouting(t *testing.T) {
	ds := newFakeBugsink(t).datasource(t)
	notFound := `{"message":"Not found."}`
	if r := callResource(t, ds, "GET", "other"); r.status != 404 || strings.TrimSpace(r.body) != notFound {
		t.Fatalf("GET other: status %d body %s", r.status, r.body)
	}
	if r := callResource(t, ds, "GET", "issues/X-1/other"); r.status != 404 || strings.TrimSpace(r.body) != notFound {
		t.Fatalf("GET issues/X-1/other: status %d body %s", r.status, r.body)
	}
	for _, path := range []string{"projects", "issues/X-1", "issues/X-1/tags", "issues/X-1/history"} {
		sender := &collectingSender{}
		req := &backend.CallResourceRequest{Method: "POST", Path: path}
		if err := ds.CallResource(context.Background(), req, sender); err != nil {
			t.Fatal(err)
		}
		r := sender.reply
		if r.status != 405 || strings.TrimSpace(r.body) != `{"message":"Method not allowed."}` {
			t.Fatalf("POST %s: status %d body %s", path, r.status, r.body)
		}
		if allow := sender.allow; allow != "GET" {
			t.Fatalf("POST %s: Allow %q", path, allow)
		}
	}
}

func resourceDatasource(t *testing.T, f *fakeBugsink) *Datasource {
	return newInstance(t, f.srv.URL, secretToken, `{"browserUrl":"http://localhost:8800"}`)
}

func TestIssueResource(t *testing.T) {
	f := newFakeBugsink(t)
	r := callResource(t, resourceDatasource(t, f), "GET", "issues/SAMPLE-PYTHON-2")
	if r.status != 200 {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(r.body), &body); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	wantKeys := []string{"bugsinkUrl", "events", "firstSeen", "friendlyId", "id", "isMuted", "isResolved",
		"isResolvedByNextRelease", "lastSeen", "message", "project", "projectId", "status", "storedEvents",
		"transaction", "type", "unmuteAfter"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys %v, want %v", keys, wantKeys)
	}
	want := map[string]any{
		"project": "alpha", "status": "open", "unmuteAfter": nil, "friendlyId": "SAMPLE-PYTHON-2",
		"bugsinkUrl": "http://localhost:8800/issues/issue/0b1e1c9e-1a2b-4c3d-8e4f-0123456789ab/event/last/",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
}

func TestIssueTagsResource(t *testing.T) {
	f := newFakeBugsink(t)
	tags := []string{
		tagResult(1, "environment", "staging", 3),
		tagResult(2, "environment", "development", 26),
	}
	for i := 6; i >= 0; i-- {
		tags = append(tags, tagResult(10+i, "trace", fmt.Sprintf("t%d", i), 1))
	}
	tags = append(tags, tagResult(30, "release", "1.0", 1))
	f.tagsBody = strings.Join(tags, ",")
	r := callResource(t, resourceDatasource(t, f), "GET", "issues/SAMPLE-PYTHON-2/tags")
	var body struct {
		Tags []struct {
			Key    string `json:"key"`
			Values []struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"values"`
			OtherValues int `json:"otherValues"`
		} `json:"tags"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(r.body), &body); err != nil || r.status != 200 {
		t.Fatalf("status %d body %s err %v", r.status, r.body, err)
	}
	if len(body.Tags) != 3 || body.Tags[0].Key != "environment" || body.Tags[1].Key != "trace" || body.Tags[2].Key != "release" {
		t.Fatalf("tags %+v", body.Tags)
	}
	env := body.Tags[0]
	if len(env.Values) != 2 || env.Values[0].Value != "development" || env.Values[0].Count != 26 ||
		env.Values[1].Value != "staging" || env.Values[1].Count != 3 || env.OtherValues != 0 {
		t.Fatalf("environment %+v", env)
	}
	trace := body.Tags[1]
	if len(trace.Values) != 5 || trace.OtherValues != 2 {
		t.Fatalf("trace %+v", trace)
	}
	for i, v := range trace.Values {
		if want := fmt.Sprintf("t%d", i); v.Value != want {
			t.Fatalf("trace value %d is %s, want %s (ties by value)", i, v.Value, want)
		}
	}
	if rel := body.Tags[2]; len(rel.Values) != 1 || rel.OtherValues != 0 {
		t.Fatalf("release %+v", rel)
	}
	if body.Truncated {
		t.Fatal("truncated")
	}

	f.tagsEndless = true
	r = callResource(t, resourceDatasource(t, f), "GET", "issues/SAMPLE-PYTHON-2/tags")
	if !strings.Contains(r.body, `"truncated":true`) {
		t.Fatalf("body %s", r.body)
	}
}

func TestIssueTagsResourceEmpty(t *testing.T) {
	f := newFakeBugsink(t)
	r := callResource(t, resourceDatasource(t, f), "GET", "issues/SAMPLE-PYTHON-2/tags")
	if r.status != 200 || strings.TrimSpace(r.body) != `{"tags":[],"truncated":false}` {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
}

func tagResult(id int, key, value string, count int) string {
	return fmt.Sprintf(`{"id":%d,"key":%q,"value":%q,"count":%d}`, id, key, value, count)
}

func TestIssueHistoryResource(t *testing.T) {
	f := newFakeBugsink(t)
	f.historyBody = `{"id":2,"kind":"manual_annotation","timestamp":"2026-05-02T10:00:00Z","user_display_name":"Ada","comment":"looked"},` +
		`{"id":1,"kind":"first_seen","timestamp":"2026-05-01T10:00:00Z","user_display_name":null,"comment":""}`
	r := callResource(t, resourceDatasource(t, f), "GET", "issues/SAMPLE-PYTHON-2/history")
	var body struct {
		History []map[string]any `json:"history"`
	}
	if err := json.Unmarshal([]byte(r.body), &body); err != nil || r.status != 200 || len(body.History) != 2 {
		t.Fatalf("status %d body %s err %v", r.status, r.body, err)
	}
	first, second := body.History[0], body.History[1]
	if first["id"] != float64(2) || first["kind"] != "manual_annotation" || first["user"] != "Ada" ||
		first["comment"] != "looked" || first["timestamp"] != "2026-05-02T10:00:00Z" {
		t.Fatalf("first %v", first)
	}
	if second["id"] != float64(1) || second["user"] != nil {
		t.Fatalf("second %v", second)
	}
	if !strings.Contains(r.body, `"truncated":false`) {
		t.Fatalf("body %s", r.body)
	}
}

func TestIssueHistoryResourceEmpty(t *testing.T) {
	f := newFakeBugsink(t)
	r := callResource(t, resourceDatasource(t, f), "GET", "issues/SAMPLE-PYTHON-2/history")
	if r.status != 200 || strings.TrimSpace(r.body) != `{"history":[],"truncated":false}` {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
}

func TestIssueRoutesRejectBadIDs(t *testing.T) {
	cases := []struct {
		path    string
		status  int
		message string
	}{
		{"issues/a.b", 400, `"a.b" is not an issue id.`},
		{"issues/a.b/tags", 400, `"a.b" is not an issue id.`},
		{"issues/a%20b/history", 400, `"a b" is not an issue id.`},
		{"issues/..", 400, `".." is not an issue id.`},
		{"issues/../projects", 400, `".." is not an issue id.`},
		{"issues/%2e%2e", 400, `".." is not an issue id.`},
		{"issues/a%2Fb", 400, `"a/b" is not an issue id.`},
		{"issues//tags", 400, `"" is not an issue id.`},
		{"issues/", 400, `"" is not an issue id.`},
		{"issues/.", 400, `"." is not an issue id.`},
		{"../projects", 404, "Not found."},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := callResource(t, resourceDatasource(t, f), "GET", c.path)
			if r.status != c.status || messageOf(t, r) != c.message {
				t.Fatalf("status %d body %s, want %d %s", r.status, r.body, c.status, c.message)
			}
			if n := f.requestCount(); n != 0 {
				t.Fatalf("%d Bugsink requests", n)
			}
		})
	}
}

func TestIssueRoutesErrors(t *testing.T) {
	cases := []struct {
		name, path string
		status     int
		body       string
		message    string
	}{
		{"refused", "issues/X-1", 403, `{"detail":"This token is not allowed to access this project."}`,
			"Bugsink refused the request: This token is not allowed to access this project."},
		{"capability", "issues/X-1/tags", 403, `{"detail":"This token does not have the required capability: events:read."}`,
			"The API token lacks events:read."},
		{"unauthorized", "issues/X-1/history", 401, `{"detail":"Invalid token."}`, "Bugsink rejected the API token."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.detailStatus, f.detailError = c.status, c.body
			r := callResource(t, resourceDatasource(t, f), "GET", c.path)
			if r.status != 502 || messageOf(t, r) != c.message || strings.Contains(r.body, secretToken) {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
		})
	}
	for _, path := range []string{"issues/X-1", "issues/X-1/tags", "issues/X-1/history"} {
		t.Run("settings "+path, func(t *testing.T) {
			r := callResource(t, newInstance(t, "", secretToken, ""), "GET", path)
			if r.status != 400 || messageOf(t, r) != msgEnterURL {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
		})
	}
}

func TestConcurrentResourceCalls(t *testing.T) {
	f := newFakeBugsink(t)
	ds := resourceDatasource(t, f)
	var wg sync.WaitGroup
	statuses := make([]int, 10)
	for i := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sender := &collectingSender{}
			req := &backend.CallResourceRequest{Method: "GET", Path: "issues/X-1"}
			if err := ds.CallResource(context.Background(), req, sender); err == nil && sender.reply != nil {
				statuses[i] = sender.reply.status
			}
		}()
	}
	wg.Wait()
	for i, s := range statuses {
		if s != 200 {
			t.Fatalf("call %d answered %d", i, s)
		}
	}
}

func TestIssueRoutesNotCached(t *testing.T) {
	f := newFakeBugsink(t)
	ds := resourceDatasource(t, f)
	callResource(t, ds, "GET", "issues/X-1")
	callResource(t, ds, "GET", "issues/X-1")
	if n := f.pathCount("/api/canonical/0/issues/X-1/"); n != 2 {
		t.Fatalf("%d Bugsink requests, want 2", n)
	}
}

func TestProjectsResourceCache(t *testing.T) {
	f := newFakeBugsink(t)
	ds := f.datasource(t)
	callResource(t, ds, "GET", "projects")
	callResource(t, ds, "GET", "projects")
	if f.projectCalls != 1 {
		t.Fatalf("%d project requests, want 1", f.projectCalls)
	}
}

func eventFixture(t *testing.T, name string) (id, body string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "bugsink", "testdata", "events", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var head struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		t.Fatal(err)
	}
	return head.ID, string(raw)
}

func eventResource(t *testing.T, fixture string) (resourceReply, map[string]any) {
	t.Helper()
	f := newFakeBugsink(t)
	id, body := eventFixture(t, fixture)
	f.eventDetails = map[string]string{id: body}
	r := callResource(t, resourceDatasource(t, f), "GET", "events/"+id)
	if r.status != 200 {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(r.body), &got); err != nil {
		t.Fatal(err)
	}
	return r, got
}

func TestEventResource(t *testing.T) {
	_, body := eventResource(t, "python-unhandled")
	var keys []string
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	wantKeys := []string{"breadcrumbs", "bugsinkUrl", "contexts", "environment", "eventId", "exceptions", "id",
		"ingestedAt", "issueId", "level", "links", "message", "notices", "platform", "project", "projectId", "release",
		"stacktraceMd", "tags", "timestamp", "traceId", "transaction"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys %v, want %v", keys, wantKeys)
	}
	issue := "5f2241c0-3705-4514-975f-a53ff7c3a899"
	id := "7b5d5499-1cf8-4734-8381-14cfb03da60e"
	want := map[string]any{
		"project": "alpha", "id": id, "issueId": issue, "stacktraceMd": nil,
		"bugsinkUrl": "http://localhost:8800/issues/issue/" + issue + "/event/" + id + "/",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %v, want %v", k, body[k], v)
		}
	}
	exceptions := body["exceptions"].([]any)
	frames := exceptions[0].(map[string]any)["frames"].([]any)
	frame := frames[0].(map[string]any)
	if _, ok := frame["lineno"].(float64); !ok || frame["colno"] != nil {
		t.Errorf("frame lineno/colno = %v/%v", frame["lineno"], frame["colno"])
	}
	if notices, ok := body["notices"].([]any); !ok || len(notices) != 0 {
		t.Errorf("notices = %v", body["notices"])
	}
	exception := exceptions[0].(map[string]any)
	assertKeys(t, "exception", exception, "type", "value", "module", "mechanism", "handled", "frames", "framesOmitted")
	if exception["handled"] != false {
		t.Errorf("handled = %v", exception["handled"])
	}
	assertKeys(t, "frame", frame, "filename", "function", "module", "lineno", "colno", "inApp", "preContext", "contextLine", "postContext")
	if frame["inApp"] != false || frames[len(frames)-1].(map[string]any)["inApp"] != true {
		t.Errorf("inApp first %v last %v", frame["inApp"], frames[len(frames)-1].(map[string]any)["inApp"])
	}
	crumb := body["breadcrumbs"].([]any)[0].(map[string]any)
	assertKeys(t, "breadcrumb", crumb, "timestamp", "type", "category", "level", "message", "data")
	if ts, ok := crumb["timestamp"].(string); !ok {
		t.Errorf("timestamp = %v", crumb["timestamp"])
	} else if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("timestamp %q: %v", ts, err)
	}
	assertKeys(t, "context", body["contexts"].([]any)[0].(map[string]any), "name", "fields")
	assertKeys(t, "tag", body["tags"].([]any)[0].(map[string]any), "key", "value")
}

func assertKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s keys %v, want %v", what, got, want)
	}
}

func TestEventResourceEmptyLists(t *testing.T) {
	r, body := eventResource(t, "message-only")
	if body["project"] != "2" {
		t.Errorf("project = %v", body["project"])
	}
	for _, key := range []string{"tags", "breadcrumbs", "contexts", "exceptions"} {
		if list, ok := body[key].([]any); !ok || len(list) != 0 {
			t.Errorf("%s = %v in %s", key, body[key], r.body)
		}
	}
}

func TestEventNotices(t *testing.T) {
	e := bugsink.EventDetail{
		Exceptions: []bugsink.Exception{
			{Type: "ValueError", FramesOmitted: 3},
			{Type: "KeyError"},
			{Type: "OSError", FramesOmitted: 1},
		},
		ExceptionsCut: true, BreadcrumbsCut: true, EntriesCut: true, ValuesCut: true, StacktraceMDCut: true,
	}
	want := []string{
		"Only the last 10 exceptions of the chain are shown.",
		"Only the 250 frames nearest the error are shown for ValueError.",
		"Only the 250 frames nearest the error are shown for OSError.",
		"Only the newest 100 breadcrumbs are shown.",
		"Only the first 200 tags, contexts or fields of each list are shown.",
		"Values longer than 1024 characters are cut.",
		"The stack trace text is cut at 100 KB.",
	}
	if got := eventNotices(e); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := eventNotices(bugsink.EventDetail{}); len(got) != 0 {
		t.Fatalf("got %q for an event without cuts", got)
	}
}

func TestEventRouteRejectsBadIDs(t *testing.T) {
	cases := []struct {
		path    string
		status  int
		message string
	}{
		{"events/nosuch", 400, `"nosuch" is not an event id.`},
		{"events/X-1", 400, `"X-1" is not an event id.`},
		{"events/..", 400, `".." is not an event id.`},
		{"events/../projects", 400, `".." is not an event id.`},
		{"events/", 400, `"" is not an event id.`},
		{"events/urn:uuid:" + defaultEventID, 400, `"urn:uuid:` + defaultEventID + `" is not an event id.`},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			f := newFakeBugsink(t)
			r := callResource(t, resourceDatasource(t, f), "GET", c.path)
			if r.status != c.status || messageOf(t, r) != c.message {
				t.Fatalf("status %d body %s, want %d %s", r.status, r.body, c.status, c.message)
			}
			if n := f.requestCount(); n != 0 {
				t.Fatalf("%d Bugsink requests", n)
			}
		})
	}
}

func TestEventRouteErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{"unknown event", 404, `{"detail":"No Event matches the given query."}`,
			"Bugsink refused the request: No Event matches the given query."},
		{"capability", 403, `{"detail":"This token does not have the required capability: events:read."}`,
			"The API token lacks events:read."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeBugsink(t)
			f.eventStatus, f.eventError = c.status, c.body
			r := callResource(t, resourceDatasource(t, f), "GET", "events/"+defaultEventID)
			if r.status != 502 || messageOf(t, r) != c.message || strings.Contains(r.body, secretToken) {
				t.Fatalf("status %d body %s", r.status, r.body)
			}
		})
	}

	t.Run("over the limit", func(t *testing.T) {
		f := newFakeBugsink(t)
		f.eventDetails = map[string]string{defaultEventID: `{"data":"` + strings.Repeat("x", 11<<20) + `"}`}
		r := callResource(t, resourceDatasource(t, f), "GET", "events/"+defaultEventID)
		if r.status != 502 || messageOf(t, r) != "The answer from Bugsink is larger than 10 MB." {
			t.Fatalf("status %d body %.200s", r.status, r.body)
		}
	})

	t.Run("method", func(t *testing.T) {
		sender := &collectingSender{}
		req := &backend.CallResourceRequest{Method: "POST", Path: "events/" + defaultEventID}
		if err := newFakeBugsink(t).datasource(t).CallResource(context.Background(), req, sender); err != nil {
			t.Fatal(err)
		}
		if sender.reply.status != 405 || sender.allow != "GET" {
			t.Fatalf("status %d allow %q", sender.reply.status, sender.allow)
		}
	})

	t.Run("settings", func(t *testing.T) {
		r := callResource(t, newInstance(t, "", secretToken, ""), "GET", "events/"+defaultEventID)
		if r.status != 400 || messageOf(t, r) != msgEnterURL {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
	})
}

func eventLinks(t *testing.T, fixture, jsonData string) any {
	t.Helper()
	return eventLinksWithTrace(t, fixture, jsonData, "")
}

// eventLinksWithTrace answers the event's links, with the fixture's trace id replaced when traceID is not empty.
func eventLinksWithTrace(t *testing.T, fixture, jsonData, traceID string) any {
	t.Helper()
	f := newFakeBugsink(t)
	id, body := eventFixture(t, fixture)
	if traceID != "" {
		body = strings.ReplaceAll(body, "97dc67918c526f3d407fdcec08e956da", traceID)
	}
	f.eventDetails = map[string]string{id: body}
	ds := newInstance(t, f.srv.URL, secretToken, jsonData)
	r := callResource(t, ds, "GET", "events/"+id)
	if r.status != 200 {
		t.Fatalf("status %d body %s", r.status, r.body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(r.body), &got); err != nil {
		t.Fatal(err)
	}
	return got["links"]
}

func TestEventResourceLinks(t *testing.T) {
	const both = `{"tempoDatasourceUid":"tempo","lokiDatasourceUid":"loki"}`
	rng := map[string]any{"from": "2026-11-13T23:14:57.360302Z", "to": "2026-11-13T23:44:57.360302Z"}
	want := []any{
		map[string]any{"title": "Open trace", "internal": map[string]any{
			"query":         map[string]any{"refId": "A", "queryType": "traceId", "query": "97dc67918c526f3d407fdcec08e956da"},
			"datasourceUid": "tempo", "timeRange": rng}},
		map[string]any{"title": "Open logs", "internal": map[string]any{
			"query": map[string]any{"refId": "A", "queryType": "range",
				"expr": `{service_name="sample-python"} | trace_id="97dc67918c526f3d407fdcec08e956da"`},
			"datasourceUid": "loki", "timeRange": rng}},
	}
	if got := eventLinks(t, "python-unhandled", both); !reflect.DeepEqual(got, want) {
		t.Fatalf("links %#v, want %#v", got, want)
	}
	for name, c := range map[string]struct{ fixture, jsonData string }{
		"no uids":     {"python-unhandled", `{}`},
		"no trace id": {"message-only", both},
	} {
		if got := eventLinks(t, c.fixture, c.jsonData); !reflect.DeepEqual(got, []any{}) {
			t.Errorf("%s: links %#v, want []", name, got)
		}
	}
}

func TestEventResourceMalformedTraceIDHasNoLinks(t *testing.T) {
	const both = `{"tempoDatasourceUid":"tempo","lokiDatasourceUid":"loki"}`
	for _, id := range []string{"${__user.login}", "97DC67918C526F3D407FDCEC08E956DA", "97dc67918c526f3d"} {
		if got := eventLinksWithTrace(t, "python-unhandled", both, id); !reflect.DeepEqual(got, []any{}) {
			t.Errorf("trace id %q: links %#v, want []", id, got)
		}
	}
}

func TestEventHidesStacktraceText(t *testing.T) {
	const hidden = "Bugsink's stack trace text is hidden for this data source."
	var getBody func(t *testing.T, id, body, jsonData string) (string, map[string]any)
	get := func(t *testing.T, fixture, jsonData string) (string, map[string]any) {
		t.Helper()
		id, body := eventFixture(t, fixture)
		return getBody(t, id, body, jsonData)
	}
	getBody = func(t *testing.T, id, body, jsonData string) (string, map[string]any) {
		t.Helper()
		f := newFakeBugsink(t)
		f.eventDetails = map[string]string{id: body}
		r := callResource(t, newInstance(t, f.srv.URL, secretToken, jsonData), "GET", "events/"+id)
		if r.status != 200 {
			t.Fatalf("status %d body %s", r.status, r.body)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(r.body), &got); err != nil {
			t.Fatal(err)
		}
		return r.body, got
	}
	const on, off = `{"hideStacktraceText":true}`, `{"hideStacktraceText":false}`
	const text = "produce.py:17"

	t.Run("on, text and no frames", func(t *testing.T) {
		raw, body := get(t, "python-message-threads", on)
		if v, ok := body["stacktraceMd"]; !ok || v != nil {
			t.Errorf("stacktraceMd = %v (present %v)", v, ok)
		}
		if strings.Contains(raw, text) {
			t.Errorf("body carries the stack trace text: %.300s", raw)
		}
		if notices := fmt.Sprint(body["notices"]); notices != "["+hidden+"]" {
			t.Errorf("notices = %s", notices)
		}
	})
	t.Run("on, text over the limit", func(t *testing.T) {
		id, body := eventFixture(t, "python-message-threads")
		var doc map[string]any
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		doc["stacktrace_md"] = text + strings.Repeat("x", bugsink.MaxStacktraceMD)
		long, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		raw, got := getBody(t, id, string(long), on)
		if got["stacktraceMd"] != nil || strings.Contains(raw, text) || fmt.Sprint(got["notices"]) != "["+hidden+"]" {
			t.Errorf("stacktraceMd %.40v notices %v", got["stacktraceMd"], got["notices"])
		}
	})
	t.Run("on, no text", func(t *testing.T) {
		_, body := get(t, "python-unhandled", on)
		if notices, ok := body["notices"].([]any); !ok || len(notices) != 0 {
			t.Errorf("notices = %v", body["notices"])
		}
		if len(body["exceptions"].([]any)) == 0 {
			t.Error("frames were dropped")
		}
	})
	for name, jsonData := range map[string]string{"off": off, "absent": ""} {
		t.Run(name+", text shown", func(t *testing.T) {
			raw, body := get(t, "python-message-threads", jsonData)
			md, ok := body["stacktraceMd"].(string)
			if !ok || !strings.Contains(md, text) || !strings.Contains(raw, text) {
				t.Errorf("stacktraceMd = %v", body["stacktraceMd"])
			}
			if notices, ok := body["notices"].([]any); !ok || len(notices) != 0 {
				t.Errorf("notices = %v", body["notices"])
			}
		})
	}
}
