// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const secretToken = "secret-token-value"

func bugsinkHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer "+secretToken {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"detail":"Invalid Bearer token."}`)
		return
	}
	switch {
	case r.URL.Path == "/api/canonical/0/projects/":
		_, _ = fmt.Fprint(w, `{"next":null,"previous":null,"results":[{"id":1,"name":"sample-python"},{"id":2,"name":"sample-js"}]}`)
	case strings.HasPrefix(r.URL.Path, "/api/canonical/0/issues/"):
		if r.URL.Path != "/api/canonical/0/issues/"+absentIssueID+"/" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"detail":"No Issue matches the given query."}`)
	case r.URL.Path == "/api/canonical/0/events/":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"issue":["This field is required."]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func runCheck(t *testing.T, url, token string, timeout time.Duration) *backend.CheckHealthResult {
	t.Helper()
	ds := &Datasource{healthTimeout: timeout}
	res, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{
		PluginContext: backend.PluginContext{
			DataSourceInstanceSettings: &backend.DataSourceInstanceSettings{
				URL:                     url,
				DecryptedSecureJSONData: map[string]string{"apiToken": token},
			},
		},
	})
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	return res
}

func closedPortURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return "http://" + addr
}

func assertNoToken(t *testing.T, res *backend.CheckHealthResult) {
	t.Helper()
	if strings.Contains(res.Message, secretToken) || strings.Contains(string(res.JSONDetails), secretToken) {
		t.Fatalf("result leaks the token: %q %q", res.Message, res.JSONDetails)
	}
}

func TestCheckHealthEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(bugsinkHandler))
	defer srv.Close()
	res := runCheck(t, srv.URL, secretToken, defaultHealthTimeout)
	want := "Connected to Bugsink. Projects visible to this token: sample-python, sample-js."
	if res.Status != backend.HealthStatusOk || res.Message != want {
		t.Fatalf("got %v %q", res.Status, res.Message)
	}
	assertNoToken(t, res)
}

func TestCheckHealthSettingsErrors(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer srv.Close()
	tests := []struct{ name, url, token, message string }{
		{"empty URL", "", secretToken, "Enter the Bugsink URL."},
		{"ftp URL", "ftp://x", secretToken, "The Bugsink URL must start with http:// or https://."},
		{"empty token", srv.URL, "", "Enter an API token."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runCheck(t, tt.url, tt.token, defaultHealthTimeout)
			if res.Status != backend.HealthStatusError || res.Message != tt.message {
				t.Fatalf("got %v %q", res.Status, res.Message)
			}
			assertNoToken(t, res)
		})
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("server received %d requests", n)
	}
}

func TestCheckHealthUnreachable(t *testing.T) {
	res := runCheck(t, closedPortURL(t), secretToken, defaultHealthTimeout)
	if res.Status != backend.HealthStatusError || !strings.HasPrefix(res.Message, "Cannot reach Bugsink at http://127.0.0.1:") || !strings.HasSuffix(res.Message, ".") {
		t.Fatalf("got %v %q", res.Status, res.Message)
	}
	if strings.Contains(res.Message, "/api/") || strings.Contains(res.Message, "Get ") {
		t.Fatalf("cause carries the request: %q", res.Message)
	}
	assertNoToken(t, res)
}

func TestCheckHealthTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	res := runCheck(t, srv.URL, secretToken, 50*time.Millisecond)
	if res.Status != backend.HealthStatusError || !strings.Contains(res.Message, "did not answer within") {
		t.Fatalf("got %v %q", res.Status, res.Message)
	}
	assertNoToken(t, res)
}

func TestCheckHealthOnHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><body>hello</body></html>")
	}))
	defer srv.Close()
	res := runCheck(t, srv.URL, secretToken, defaultHealthTimeout)
	want := srv.URL + " did not answer like a Bugsink API (HTTP 200). Check the URL: it is the address of Bugsink itself, without /api."
	if res.Status != backend.HealthStatusError || res.Message != want {
		t.Fatalf("got %v %q", res.Status, res.Message)
	}
	assertNoToken(t, res)
}

func TestCheckHealthNeverLeaksToken(t *testing.T) {
	// A server that echoes the token back in every answer.
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, `{"detail":"%s"}`, r.Header.Get("Authorization"))
	}))
	defer echo.Close()
	ok := httptest.NewServer(http.HandlerFunc(bugsinkHandler))
	defer ok.Close()
	for _, url := range []string{echo.URL, ok.URL, closedPortURL(t), "ftp://x", ""} {
		assertNoToken(t, runCheck(t, url, secretToken, defaultHealthTimeout))
	}
}
