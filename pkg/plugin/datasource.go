// SPDX-License-Identifier: GPL-3.0-or-later
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// Datasource is one Bugsink data source instance.
type Datasource struct {
	client             *bugsink.Client // nil when settingsErr is set
	httpClient         *http.Client
	settingsErr        string // the health check's message for invalid settings, or empty
	url                string // the Bugsink URL as entered, for messages
	browserURL         string // base of links to Bugsink's pages, no trailing slash
	links              linkSettings
	triageEnabled      bool
	attribution        bool // add an attribution comment after each triage action
	hideStacktraceText bool
	healthTimeout      time.Duration
	issueCache         *cache[bugsink.IssueList]
	projectCache       *cache[projectsResult]
	eventCache         *cache[eventsResult]
	resources          backend.CallResourceHandler
}

var (
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

const (
	msgEnterURL         = "Enter the Bugsink URL."
	msgBugsinkURLScheme = "The Bugsink URL must start with http:// or https://."
	msgEnterToken       = "Enter an API token."
	msgSettingsInvalid  = "The data source settings are invalid."
	msgBrowserURLScheme = "The browser URL must start with http:// or https://."
)

// NewDatasource creates the instance Grafana asks for; invalid settings are reported by the health check.
func NewDatasource(ctx context.Context, s backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	d := &Datasource{
		url:           s.URL,
		healthTimeout: defaultHealthTimeout,
		issueCache:    newCache[bugsink.IssueList](cacheTTL, cacheMaxEntries),
		projectCache:  newCache[projectsResult](cacheTTL, cacheMaxEntries),
		eventCache:    newCache[eventsResult](cacheTTL, cacheMaxEntries),
	}
	d.settingsErr = d.configure(ctx, s)
	d.resources = httpadapter.New(canonical(d.routes()))
	return d, nil
}

// configure builds the client and browser URL and returns the message for invalid settings, or empty.
func (d *Datasource) configure(ctx context.Context, s backend.DataSourceInstanceSettings) string {
	if strings.TrimSpace(s.URL) == "" {
		return msgEnterURL
	}
	opts, err := s.HTTPClientOptions(ctx)
	if err != nil {
		return msgSettingsInvalid
	}
	d.httpClient, err = httpclient.New(opts)
	if err != nil {
		return msgSettingsInvalid
	}
	client, err := bugsink.New(s.URL, s.DecryptedSecureJSONData["apiToken"], d.httpClient)
	switch {
	case errors.Is(err, bugsink.ErrInvalidURL):
		return msgBugsinkURLScheme
	case errors.Is(err, bugsink.ErrNoToken):
		return msgEnterToken
	case err != nil:
		return msgSettingsInvalid
	}
	var jsonData struct {
		BrowserURL          string `json:"browserUrl"`
		TriageEnabled       bool   `json:"triageEnabled"`
		HideStacktraceText  bool   `json:"hideStacktraceText"`
		AttributionComments *bool  `json:"attributionComments"`
	}
	if len(s.JSONData) > 0 && json.Unmarshal(s.JSONData, &jsonData) != nil {
		return msgSettingsInvalid
	}
	browserURL := strings.TrimSpace(jsonData.BrowserURL)
	if browserURL == "" {
		browserURL = client.BaseURL()
	} else {
		u, err := url.Parse(browserURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(browserURL, "#") {
			return msgBrowserURLScheme
		}
	}
	links, invalid := parseLinkSettings(s.JSONData)
	if invalid != "" {
		return invalid
	}
	d.browserURL = strings.TrimRight(browserURL, "/")
	d.links = links
	d.triageEnabled = jsonData.TriageEnabled
	d.hideStacktraceText = jsonData.HideStacktraceText
	d.attribution = jsonData.AttributionComments == nil || *jsonData.AttributionComments
	d.client = client
	return ""
}

// Dispose closes the HTTP client's idle connections.
func (d *Datasource) Dispose() {
	if d.httpClient != nil {
		d.httpClient.CloseIdleConnections()
	}
}

// CheckHealth probes the configured Bugsink and reports what the token can do; it never returns an error.
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if d.settingsErr != "" {
		return errorResult("%s", d.settingsErr), nil
	}
	ctx, cancel := context.WithTimeout(ctx, d.healthTimeout)
	defer cancel()
	return checkHealth(ctx, d.url, d.healthTimeout, d.client, d.triageEnabled, d.attribution), nil
}
