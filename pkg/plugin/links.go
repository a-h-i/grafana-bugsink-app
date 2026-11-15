// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	defaultLinkWindow = 15 * time.Minute
	defaultLogsQuery  = `{service_name="${service}"} | trace_id="${trace_id}"`
	defaultServiceTag = "service"
	maxLinkWindowMin  = 1440
)

const msgLinkWindow = "The link time window must be a whole number of minutes from 1 to 1440."
const msgLogsQueryQuotes = "In the logs query, put every variable inside double quotes."

// linkSettings are the data source settings that turn trace ids into Tempo and Loki links.
type linkSettings struct {
	tempoUID, lokiUID string
	window            time.Duration
	logsQuery         string
	serviceTag        string // empty uses the project name
}

// logsVars are the values a logs query template can use.
type logsVars struct {
	TraceID, Service, Project, Environment, Release string
	Tags                                            []bugsink.KeyValue
}

// parseLinkSettings reads the link keys of jsonData with defaults applied; invalid is a health check message or empty.
func parseLinkSettings(jsonData json.RawMessage) (s linkSettings, invalid string) {
	s = linkSettings{window: defaultLinkWindow, logsQuery: defaultLogsQuery, serviceTag: defaultServiceTag}
	if len(jsonData) == 0 {
		return s, ""
	}
	var raw struct {
		TempoUID   string          `json:"tempoDatasourceUid"`
		LokiUID    string          `json:"lokiDatasourceUid"`
		Window     json.RawMessage `json:"linkWindowMinutes"`
		LogsQuery  *string         `json:"logsQuery"`
		ServiceTag *string         `json:"serviceTag"`
	}
	if json.Unmarshal(jsonData, &raw) != nil {
		return s, msgSettingsInvalid
	}
	s.tempoUID, s.lokiUID = raw.TempoUID, raw.LokiUID
	if raw.LogsQuery != nil && strings.TrimSpace(*raw.LogsQuery) != "" {
		s.logsQuery = *raw.LogsQuery
		if !variablesQuoted(s.logsQuery) {
			return s, msgLogsQueryQuotes
		}
	}
	if raw.ServiceTag != nil {
		s.serviceTag = *raw.ServiceTag
	}
	if len(raw.Window) > 0 && string(raw.Window) != "null" {
		var minutes float64
		if json.Unmarshal(raw.Window, &minutes) != nil || minutes != math.Floor(minutes) || minutes < 1 || minutes > maxLinkWindowMin {
			return s, msgLinkWindow
		}
		s.window = time.Duration(minutes) * time.Minute
	}
	return s, ""
}

// traceIDPattern is the form of a trace id that gets Tempo and Loki links.
var traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

var logsVarPattern = regexp.MustCompile(`\$\{([^}\n]*)\}`)

// variablesQuoted reports whether every substituted variable of the template sits inside a closed double-quoted string and outside comments; a newline ends a double-quoted string.
func variablesQuoted(template string) bool {
	inQuote, inRaw, open := false, false, 0
	for i := 0; i < len(template); i++ {
		switch c := template[i]; {
		case inQuote && c == '\\':
			if substitutedLen(template[i+1:]) > 0 {
				return false
			}
			i++
		case inQuote && c == '"':
			inQuote, open = false, 0
		case inQuote && c == '\n':
			if open > 0 {
				return false
			}
			inQuote = false
		case !inQuote && !inRaw && c == '"':
			inQuote = true
		case !inQuote && c == '`':
			inRaw = !inRaw
		case !inQuote && !inRaw && (c == '#' || c == '/'):
			n := commentLen(template[i:])
			for _, m := range logsVarPattern.FindAllStringSubmatch(template[i:i+n], -1) {
				if isSubstituted(m[1]) {
					return false
				}
			}
			i += max(n-1, 0)
		case c == '$':
			if n := substitutedLen(template[i:]); n > 0 {
				if !inQuote {
					return false
				}
				open++
				i += n - 1
			}
		}
	}
	return open == 0
}

// commentLen is the length of the #, // or /* */ comment at the start of s (an unclosed /* runs to the end), or 0 when s starts none.
func commentLen(s string) int {
	switch {
	case strings.HasPrefix(s, "#"), strings.HasPrefix(s, "//"):
		if n := strings.IndexByte(s, '\n'); n >= 0 {
			return n
		}
	case strings.HasPrefix(s, "/*"):
		if n := strings.Index(s[2:], "*/"); n >= 0 {
			return n + 4
		}
	default:
		return 0
	}
	return len(s)
}

// substitutedLen is the length of the substituted variable at the start of s, or 0 when there is none.
func substitutedLen(s string) int {
	loc := logsVarPattern.FindStringIndex(s)
	if loc == nil || loc[0] != 0 || !isSubstituted(s[2:loc[1]-1]) {
		return 0
	}
	return loc[1]
}

// isSubstituted reports whether renderLogsQuery replaces the variable name with an event value.
func isSubstituted(name string) bool {
	switch name {
	case "trace_id", "service", "project", "environment", "release":
		return true
	}
	return strings.HasPrefix(name, "tag:")
}

// renderLogsQuery fills the template in one pass, escaping each value for a LogQL string literal; unknown variables stay as written.
func renderLogsQuery(template string, v logsVars) string {
	return logsVarPattern.ReplaceAllStringFunc(template, func(match string) string {
		name := match[2 : len(match)-1]
		switch name {
		case "trace_id":
			return escapeLogQL(v.TraceID)
		case "service":
			return escapeLogQL(v.Service)
		case "project":
			return escapeLogQL(v.Project)
		case "environment":
			return escapeLogQL(v.Environment)
		case "release":
			return escapeLogQL(v.Release)
		}
		if key, ok := strings.CutPrefix(name, "tag:"); ok {
			return escapeLogQL(firstTagValue(v.Tags, key))
		}
		return match
	})
}

// escapeLogQL escapes s for the inside of a double-quoted LogQL string; $ and [ are written as \u escapes so Grafana's template interpolation finds none.
func escapeLogQL(s string) string {
	q := strconv.Quote(s)
	q = strings.ReplaceAll(q[1:len(q)-1], "$", `\u0024`)
	return strings.ReplaceAll(q, "[", `\u005b`)
}

// firstTagValue returns the value of the first tag with the key, or "" when there is none.
func firstTagValue(tags []bugsink.KeyValue, key string) string {
	for _, t := range tags {
		if t.Key == key {
			return t.Value
		}
	}
	return ""
}

// vars collects the template values of a trace in a project; e is nil when only the trace id and project are known.
func (s linkSettings) vars(traceID, project string, e *bugsink.EventDetail) logsVars {
	v := logsVars{TraceID: traceID, Service: project, Project: project}
	if e == nil {
		return v
	}
	v.Environment, v.Release, v.Tags = e.Environment, e.Release, e.Tags
	if s.serviceTag != "" {
		if service := firstTagValue(e.Tags, s.serviceTag); service != "" {
			v.Service = service
		}
	}
	return v
}

// exploreLinks returns the Tempo link, then the Loki link, each only when its data source is set.
func (s linkSettings) exploreLinks(traceQuery, logsExpr string, from, to time.Time) []data.DataLink {
	rng := data.TimeRange{From: from, To: to}
	var links []data.DataLink
	if s.tempoUID != "" {
		links = append(links, data.DataLink{
			Title: "Open trace",
			Internal: &data.InternalDataLink{
				DatasourceUID: s.tempoUID,
				Query:         map[string]any{"refId": "A", "queryType": "traceId", "query": traceQuery},
				Range:         &rng,
			},
		})
	}
	if s.lokiUID != "" {
		links = append(links, data.DataLink{
			Title: "Open logs",
			Internal: &data.InternalDataLink{
				DatasourceUID: s.lokiUID,
				Query:         map[string]any{"refId": "A", "queryType": "range", "expr": logsExpr},
				Range:         &rng,
			},
		})
	}
	return links
}
