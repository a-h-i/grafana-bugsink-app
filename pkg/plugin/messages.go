// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// userMessage renders an error from the Bugsink client as the message contract's text and status.
func userMessage(err error) (message string, status backend.Status) {
	var apiErr *bugsink.APIError
	switch {
	case isTimeout(err):
		return "Bugsink did not answer in time.", backend.StatusTimeout
	case errors.Is(err, bugsink.ErrTooLarge):
		return "The answer from Bugsink is larger than 10 MB.", backend.StatusBadGateway
	case errors.Is(err, bugsink.ErrNotBugsink):
		return "The answer did not look like a Bugsink API.", backend.StatusBadGateway
	case !errors.As(err, &apiErr):
		return fmt.Sprintf("Cannot reach Bugsink: %s.", rootCause(err)), backend.StatusBadGateway
	case apiErr.Status == 401:
		return "Bugsink rejected the API token.", backend.StatusBadGateway
	case apiErr.Capability != "":
		return fmt.Sprintf("The API token lacks %s.", apiErr.Capability), backend.Status(apiErr.Status)
	case apiErr.Detail != "":
		return "Bugsink refused the request: " + apiErr.Detail, backend.Status(apiErr.Status)
	default:
		return fmt.Sprintf("Bugsink answered HTTP %d.", apiErr.Status), backend.StatusBadGateway
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout())
}
