// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func newInstance(t *testing.T, url, token, jsonData string) *Datasource {
	t.Helper()
	settings := backend.DataSourceInstanceSettings{
		URL:                     url,
		DecryptedSecureJSONData: map[string]string{"apiToken": token},
	}
	if jsonData != "" {
		settings.JSONData = []byte(jsonData)
	}
	inst, err := NewDatasource(context.Background(), settings)
	if err != nil {
		t.Fatalf("NewDatasource returned error: %v", err)
	}
	ds := inst.(*Datasource)
	t.Cleanup(ds.Dispose)
	return ds
}

func check(t *testing.T, ds *Datasource) *backend.CheckHealthResult {
	t.Helper()
	res, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	return res
}

func runCheck(t *testing.T, url, token string, timeout time.Duration) *backend.CheckHealthResult {
	t.Helper()
	ds := newInstance(t, url, token, "")
	ds.healthTimeout = timeout
	return check(t, ds)
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

func TestBrowserURL(t *testing.T) {
	tests := []struct{ name, jsonData, want string }{
		{"empty gives the base URL", "", "http://bugsink.example:8000"},
		{"empty field gives the base URL", `{"browserUrl":""}`, "http://bugsink.example:8000"},
		{"trailing slash trimmed", `{"browserUrl":"http://localhost:8800/"}`, "http://localhost:8800"},
		{"https kept", `{"browserUrl":"https://bugsink.example/sub"}`, "https://bugsink.example/sub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := newInstance(t, "http://bugsink.example:8000/", secretToken, tt.jsonData)
			if ds.settingsErr != "" {
				t.Fatalf("settingsErr = %q", ds.settingsErr)
			}
			if ds.browserURL != tt.want {
				t.Fatalf("browserURL = %q, want %q", ds.browserURL, tt.want)
			}
		})
	}
}

func TestInvalidBrowserURL(t *testing.T) {
	for _, browserURL := range []string{"javascript:alert(1)", "localhost:8800", "http://u:p@localhost:8800", "http://localhost:8800/?x=1", "http://localhost:8800/#y"} {
		t.Run(browserURL, func(t *testing.T) {
			ds := newInstance(t, "http://bugsink.example", secretToken, fmt.Sprintf(`{"browserUrl":%q}`, browserURL))
			res := check(t, ds)
			if res.Status != backend.HealthStatusError || res.Message != "The browser URL must start with http:// or https://." {
				t.Fatalf("got %v %q", res.Status, res.Message)
			}
			if ds.client != nil {
				t.Fatal("client is set while the settings are invalid")
			}
		})
	}
}

func TestMalformedJSONDataLeavesNoClient(t *testing.T) {
	ds := newInstance(t, "http://bugsink.example", secretToken, `{"browserUrl":`)
	if ds.settingsErr == "" || ds.client != nil {
		t.Fatalf("settingsErr = %q, client set = %v", ds.settingsErr, ds.client != nil)
	}
}

func TestSettingsErrorsTakePrecedenceOverBrowserURL(t *testing.T) {
	ds := newInstance(t, "", secretToken, `{"browserUrl":"nope"}`)
	if res := check(t, ds); res.Message != "Enter the Bugsink URL." {
		t.Fatalf("got %q", res.Message)
	}
}

func TestHealthChecksShareOneConnection(t *testing.T) {
	var opened atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(bugsinkHandler))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			opened.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	ds := newInstance(t, srv.URL, secretToken, "")
	for range 2 {
		if res := check(t, ds); res.Status != backend.HealthStatusOk {
			t.Fatalf("got %v %q", res.Status, res.Message)
		}
	}
	if n := opened.Load(); n != 1 {
		t.Fatalf("opened %d connections, want 1", n)
	}
}

func TestLinkSettingsInHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(bugsinkHandler))
	defer srv.Close()
	ok := newInstance(t, srv.URL, secretToken, `{"browserUrl":"http://b"}`)
	if res := check(t, ok); res.Status != backend.HealthStatusOk {
		t.Fatalf("no link keys: %v %q", res.Status, res.Message)
	}
	if ok.links.window != defaultLinkWindow || ok.links.logsQuery != defaultLogsQuery {
		t.Fatalf("links = %+v", ok.links)
	}
	bad := newInstance(t, srv.URL, secretToken, `{"linkWindowMinutes":0,"lokiDatasourceUid":"loki"}`)
	if res := check(t, bad); res.Status != backend.HealthStatusError || res.Message != msgLinkWindow {
		t.Fatalf("window 0: %v %q", res.Status, res.Message)
	}
	unquoted := newInstance(t, srv.URL, secretToken, `{"logsQuery":"{service_name=`+"`${service}`"+`}"}`)
	if res := check(t, unquoted); res.Status != backend.HealthStatusError || res.Message != msgLogsQueryQuotes {
		t.Fatalf("unquoted variable: %v %q", res.Status, res.Message)
	}
	set := newInstance(t, srv.URL, secretToken, `{"lokiDatasourceUid":"loki","tempoDatasourceUid":"tempo"}`)
	if set.links.lokiUID != "loki" || set.links.tempoUID != "tempo" {
		t.Fatalf("links = %+v", set.links)
	}
}

func TestSwitches(t *testing.T) {
	tests := []struct {
		name, jsonData string
		triage, hide   bool
	}{
		{"absent", `{"browserUrl":"http://b"}`, false, false},
		{"no jsonData", "", false, false},
		{"triage on", `{"triageEnabled":true}`, true, false},
		{"triage false", `{"triageEnabled":false}`, false, false},
		{"triage null", `{"triageEnabled":null}`, false, false},
		{"hide on", `{"hideStacktraceText":true}`, false, true},
		{"both on", `{"triageEnabled":true,"hideStacktraceText":true}`, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := newInstance(t, "http://bugsink.example", secretToken, tt.jsonData)
			if ds.settingsErr != "" {
				t.Fatalf("settingsErr = %q", ds.settingsErr)
			}
			if ds.triageEnabled != tt.triage || ds.hideStacktraceText != tt.hide {
				t.Fatalf("triageEnabled %v hideStacktraceText %v, want %v %v", ds.triageEnabled, ds.hideStacktraceText, tt.triage, tt.hide)
			}
		})
	}
}

func TestSwitchesOfTheWrongType(t *testing.T) {
	for _, jsonData := range []string{
		`{"triageEnabled":"yes"}`, `{"triageEnabled":"true"}`, `{"triageEnabled":1}`, `{"triageEnabled":{}}`,
		`{"hideStacktraceText":"yes"}`, `{"hideStacktraceText":1}`, `{"hideStacktraceText":[true]}`,
	} {
		t.Run(jsonData, func(t *testing.T) {
			ds := newInstance(t, "http://bugsink.example", secretToken, jsonData)
			if res := check(t, ds); res.Status != backend.HealthStatusError || res.Message != "The data source settings are invalid." {
				t.Fatalf("got %v %q", res.Status, res.Message)
			}
			if ds.client != nil || ds.triageEnabled || ds.hideStacktraceText {
				t.Fatalf("client set %v, triageEnabled %v, hideStacktraceText %v", ds.client != nil, ds.triageEnabled, ds.hideStacktraceText)
			}
		})
	}
}

// triageServer answers like bugsinkHandler and records every POST; triage answers the POSTs.
func triageServer(t *testing.T, triage http.HandlerFunc) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var posts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			bugsinkHandler(w, r)
			return
		}
		mu.Lock()
		posts = append(posts, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		triage(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &posts
}

func TestCheckHealthTriageEndToEnd(t *testing.T) {
	absent := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"detail":"No Issue matches the given query."}`)
	}
	lacking := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"detail":"This token does not have the required capability: issues:triage."}`)
	}
	connected := "Connected to Bugsink. Projects visible to this token: sample-python, sample-js."
	probePath := "/api/canonical/0/issues/" + absentIssueID + "/resolve/"
	tests := []struct {
		name, jsonData string
		triage         http.HandlerFunc
		status         backend.HealthStatus
		message        string
		posts          []string
	}{
		{"off", "", absent, backend.HealthStatusOk, connected, nil},
		{"off explicitly", `{"triageEnabled":false}`, absent, backend.HealthStatusOk, connected, nil},
		{"on, token can triage", `{"triageEnabled":true}`, absent, backend.HealthStatusOk, connected + " The token can triage issues.", []string{probePath}},
		{"on, token cannot triage", `{"triageEnabled":true}`, lacking, backend.HealthStatusError, "The API token lacks: issues:triage.", []string{probePath}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, posts := triageServer(t, tt.triage)
			res := check(t, newInstance(t, srv.URL, secretToken, tt.jsonData))
			if res.Status != tt.status || res.Message != tt.message {
				t.Fatalf("got %v %q\nwant %v %q", res.Status, res.Message, tt.status, tt.message)
			}
			if fmt.Sprint(*posts) != fmt.Sprint(tt.posts) {
				t.Fatalf("POSTs = %v, want %v", *posts, tt.posts)
			}
			assertNoToken(t, res)
		})
	}
}
