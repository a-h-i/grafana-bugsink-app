// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type resourceReply struct {
	status int
	body   string
}

type collectingSender struct {
	reply *resourceReply
	allow string
}

func (c *collectingSender) Send(r *backend.CallResourceResponse) error {
	c.reply = &resourceReply{status: r.Status, body: string(r.Body)}
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
