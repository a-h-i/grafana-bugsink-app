// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Capability is a Bugsink token capability name, such as "issues:read".
type Capability string

const (
	ProjectsRead Capability = "projects:read"
	IssuesRead   Capability = "issues:read"
	EventsRead   Capability = "events:read"
)

var (
	ErrInvalidURL = errors.New("invalid Bugsink URL")
	ErrNoToken    = errors.New("no API token")
	ErrNotBugsink = errors.New("not a Bugsink API response")
)

const missingCapabilityPrefix = "This token does not have the required capability: "

// APIError is any Bugsink answer that is not the success the call expects.
type APIError struct {
	Status     int
	Capability Capability // set when a 403 body names a missing capability
	Detail     string     // Bugsink's "detail" text, empty when the body is not Bugsink JSON
}

func (e *APIError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("Bugsink answered %d", e.Status)
	}
	return fmt.Sprintf("Bugsink answered %d: %s", e.Status, e.Detail)
}

func newAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	var parsed struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return e
	}
	e.Detail = parsed.Detail
	if status == 403 {
		if name, ok := strings.CutPrefix(parsed.Detail, missingCapabilityPrefix); ok {
			e.Capability = Capability(strings.TrimSuffix(name, "."))
		}
	}
	return e
}
