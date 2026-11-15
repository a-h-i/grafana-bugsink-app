// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Capability is a Bugsink token capability name, such as "issues:read".
type Capability string

const (
	ProjectsRead  Capability = "projects:read"
	IssuesRead    Capability = "issues:read"
	EventsRead    Capability = "events:read"
	IssuesTriage  Capability = "issues:triage"
	IssuesComment Capability = "issues:comment"
)

var (
	ErrInvalidURL = errors.New("invalid Bugsink URL")
	ErrNoToken    = errors.New("no API token")
	ErrNotBugsink = errors.New("not a Bugsink API response")
	ErrTooLarge   = errors.New("answer from Bugsink is larger than the read limit")
)

const missingCapabilityPrefix = "This token does not have the required capability: "

// maxDetailChars is the most characters of Bugsink's text an error keeps.
const maxDetailChars = 500

// redacted replaces the API token wherever Bugsink's text holds it.
const redacted = "[redacted]"

const ellipsis = "…"

// forwardable is text with every occurrence of token, in any ASCII letter case, replaced by redacted, then cut to maxDetailChars characters.
func forwardable(text, token string) string {
	return cut(redact(prefix(text, reach(token)), token))
}

// reach is how many leading characters of a text are enough to redact token from before cutting it to maxDetailChars.
func reach(token string) int {
	chars := utf8.RuneCountInString(token)
	return (maxDetailChars + chars) * max(1, (chars+len(redacted)-1)/len(redacted))
}

func redact(text, token string) string {
	if token == "" {
		return text
	}
	folded, needle := asciiLower(text), asciiLower(token)
	var b strings.Builder
	for {
		i := strings.Index(folded, needle)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		b.WriteString(text[:i])
		b.WriteString(redacted)
		text, folded = text[i+len(token):], folded[i+len(token):]
	}
}

// asciiLower lowers A to Z only, so every byte keeps its index.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// cut keeps the first maxDetailChars characters of text and marks a cut with an ellipsis.
func cut(text string) string {
	if kept := prefix(text, maxDetailChars); len(kept) < len(text) {
		return kept + ellipsis
	}
	return text
}

// prefix is the first n characters of text.
func prefix(text string, n int) string {
	chars := 0
	for i := range text {
		if chars == n {
			return text[:i]
		}
		chars++
	}
	return text
}

// APIError is any Bugsink answer that is not the success the call expects.
type APIError struct {
	Status     int
	Capability Capability // set when a 403 body names a missing capability
	Detail     string     // Bugsink's "detail" text or its field errors, empty when the body is not Bugsink JSON
}

func (e *APIError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("Bugsink answered %d", e.Status)
	}
	return fmt.Sprintf("Bugsink answered %d: %s", e.Status, e.Detail)
}

// newAPIError reads Bugsink's detail and capability from body, then redacts token from both and cuts them to maxDetailChars.
func newAPIError(status int, body []byte, token string) *APIError {
	e := &APIError{Status: status}
	var list []string
	if json.Unmarshal(body, &list) == nil {
		e.Detail = strings.Join(list, " ")
	} else if parsed := map[string]json.RawMessage{}; json.Unmarshal(body, &parsed) == nil {
		e.Detail = detailOf(parsed)
		if status == 403 {
			if name, ok := strings.CutPrefix(e.Detail, missingCapabilityPrefix); ok {
				e.Capability = Capability(strings.TrimSuffix(name, "."))
			}
		}
	}
	e.Detail = forwardable(e.Detail, token)
	e.Capability = Capability(forwardable(string(e.Capability), token))
	return e
}

// detailOf returns the "detail" text, or "<key>: <first message>" per field error in key order.
func detailOf(parsed map[string]json.RawMessage) string {
	var detail string
	if json.Unmarshal(parsed["detail"], &detail) == nil && detail != "" {
		return detail
	}
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		var msgs []string
		if json.Unmarshal(parsed[k], &msgs) == nil && len(msgs) > 0 {
			parts = append(parts, k+": "+msgs[0])
		}
	}
	return strings.Join(parts, " ")
}
