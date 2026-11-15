// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type resourceReply struct {
	status int
	body   string
}

type collectingSender struct{ reply *resourceReply }

func (c *collectingSender) Send(r *backend.CallResourceResponse) error {
	c.reply = &resourceReply{status: r.Status, body: string(r.Body)}
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
	if r := callResource(t, ds, "GET", "other"); r.status != 404 {
		t.Fatalf("GET other: status %d", r.status)
	}
	if r := callResource(t, ds, "POST", "projects"); r.status != 405 {
		t.Fatalf("POST projects: status %d", r.status)
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
