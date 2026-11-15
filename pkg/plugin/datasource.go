// SPDX-License-Identifier: GPL-3.0-or-later
package plugin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
)

// Datasource is one Bugsink data source instance.
type Datasource struct {
	healthTimeout time.Duration
}

var _ backend.CheckHealthHandler = (*Datasource)(nil)

// NewDatasource creates the instance Grafana asks for.
func NewDatasource(_ context.Context, _ backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	return &Datasource{healthTimeout: defaultHealthTimeout}, nil
}

// CheckHealth probes the configured Bugsink and reports what the token can do; it never returns an error.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	settings := req.PluginContext.DataSourceInstanceSettings
	if settings == nil || strings.TrimSpace(settings.URL) == "" {
		return errorResult("Enter the Bugsink URL."), nil
	}
	opts, err := settings.HTTPClientOptions(ctx)
	if err != nil {
		return errorResult("The data source settings are invalid."), nil
	}
	httpClient, err := httpclient.New(opts)
	if err != nil {
		return errorResult("The data source settings are invalid."), nil
	}
	defer httpClient.CloseIdleConnections()
	client, err := bugsink.New(settings.URL, settings.DecryptedSecureJSONData["apiToken"], httpClient)
	switch {
	case errors.Is(err, bugsink.ErrInvalidURL):
		return errorResult("The Bugsink URL must start with http:// or https://."), nil
	case errors.Is(err, bugsink.ErrNoToken):
		return errorResult("Enter an API token."), nil
	case err != nil:
		return errorResult("The data source settings are invalid."), nil
	}
	ctx, cancel := context.WithTimeout(ctx, d.healthTimeout)
	defer cancel()
	return checkHealth(ctx, settings.URL, d.healthTimeout, client), nil
}
