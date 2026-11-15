// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const timeoutMessage = "Bugsink did not answer in time."

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestUserMessage(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		message string
		status  backend.Status
	}{
		{"401", &bugsink.APIError{Status: 401, Detail: "Invalid Bearer token."}, "Bugsink rejected the API token.", 502},
		{"403 capability", &bugsink.APIError{Status: 403, Capability: bugsink.IssuesRead}, "The API token lacks issues:read.", 403},
		{"403 detail", &bugsink.APIError{Status: 403, Detail: "This token is not allowed to access this project."},
			"Bugsink refused the request: This token is not allowed to access this project.", 403},
		{"404 detail", &bugsink.APIError{Status: 404, Detail: "No Project matches the given query."},
			"Bugsink refused the request: No Project matches the given query.", 404},
		{"no detail", &bugsink.APIError{Status: 500}, "Bugsink answered HTTP 500.", 502},
		{"not bugsink", fmt.Errorf("%w: %w", bugsink.ErrNotBugsink, &bugsink.APIError{Status: 200}),
			"The answer did not look like a Bugsink API.", 502},
		{"network", fmt.Errorf("Get \"http://x/?token=secret\": %w", errors.New("dial tcp 127.0.0.1:1: connect: connection refused")),
			"Cannot reach Bugsink: dial tcp 127.0.0.1:1: connect: connection refused.", 502},
		{"cancelled", context.Canceled, timeoutMessage, 504},
		{"deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), timeoutMessage, 504},
		{"client timeout", &url.Error{Op: "Get", URL: "http://x/?token=secret", Err: timeoutNetError{}}, timeoutMessage, 504},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			message, status := userMessage(c.err)
			if message != c.message || status != c.status {
				t.Fatalf("got %q %d, want %q %d", message, status, c.message, c.status)
			}
		})
	}
}
