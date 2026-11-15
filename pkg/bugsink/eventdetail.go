// SPDX-License-Identifier: GPL-3.0-or-later

package bugsink

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	MaxExceptions   = 10
	MaxFrames       = 250
	MaxContextLines = 5
	MaxBreadcrumbs  = 100
	MaxEntries      = 200
	MaxValueLength  = 1024
	MaxStacktraceMD = 100_000
)

type KeyValue struct{ Key, Value string }

type EventContext struct {
	Name   string
	Fields []KeyValue
}

type Frame struct {
	Filename, Function, Module string
	Lineno, Colno              *int
	InApp                      bool
	PreContext                 []string
	ContextLine                *string
	PostContext                []string
}

type Exception struct {
	Type, Value, Module, Mechanism string
	Handled                        *bool
	Frames                         []Frame
	FramesOmitted                  int
}

type Breadcrumb struct {
	Timestamp                      *time.Time
	Type, Category, Level, Message string
	Data                           []KeyValue
}

// EventDetail is an event with the parts of its Sentry payload the plugin shows, bounded in size.
type EventDetail struct {
	Event
	Platform, Level, Message, Transaction, Environment, Release, TraceID  string
	Tags                                                                  []KeyValue
	Contexts                                                              []EventContext
	Exceptions                                                            []Exception // protocol order
	Breadcrumbs                                                           []Breadcrumb
	StacktraceMD                                                          string // Bugsink's text, set when the frames are not parsed
	ExceptionsCut, BreadcrumbsCut, EntriesCut, ValuesCut, StacktraceMDCut bool
}

// eventBody is Bugsink's event answer with the payload left raw.
type eventBody struct {
	ID           string          `json:"id"`
	EventID      string          `json:"event_id"`
	Issue        string          `json:"issue"`
	Project      int             `json:"project"`
	Timestamp    time.Time       `json:"timestamp"`
	IngestedAt   time.Time       `json:"ingested_at"`
	Data         json.RawMessage `json:"data"`
	StacktraceMD *string         `json:"stacktrace_md"`
}

// GetEvent reads one event by Bugsink's event id and parses its payload.
func (c *Client) GetEvent(ctx context.Context, id uuid.UUID) (EventDetail, error) {
	resp, err := c.gen.EventsRetrieve(ctx, id)
	if err != nil {
		return EventDetail{}, err
	}
	var body eventBody
	if err := decodeJSON(resp, &body); err != nil {
		return EventDetail{}, err
	}
	md := ""
	if body.StacktraceMD != nil {
		md = *body.StacktraceMD
	}
	ev := parseEvent(body.Data, md)
	ev.Event = Event{
		ID: body.ID, EventID: body.EventID, IssueID: body.Issue, ProjectID: body.Project,
		Timestamp: body.Timestamp, IngestedAt: body.IngestedAt,
	}
	return ev, nil
}

// parseEvent reads the Sentry payload field by field; a field of the wrong type is left empty.
func parseEvent(data json.RawMessage, stacktraceMD string) EventDetail {
	p := &parser{}
	root := objectOf(data)
	ev := EventDetail{
		Platform:    p.str(root["platform"]),
		Level:       p.str(root["level"]),
		Message:     p.message(root),
		Transaction: p.str(root["transaction"]),
		Environment: p.str(root["environment"]),
		Release:     p.str(root["release"]),
		Tags:        p.tags(root["tags"]),
		Contexts:    p.contexts(root["contexts"]),
	}
	ev.TraceID = p.str(objectOf(objectOf(root["contexts"])["trace"])["trace_id"])
	ev.Exceptions, ev.ExceptionsCut = p.exceptions(root["exception"])
	ev.Breadcrumbs, ev.BreadcrumbsCut = p.breadcrumbs(root["breadcrumbs"])
	if !hasFrames(ev.Exceptions) && payloadHasFrames(root) {
		ev.StacktraceMD, ev.StacktraceMDCut = cutBytes(stacktraceMD, MaxStacktraceMD)
	}
	ev.EntriesCut, ev.ValuesCut = p.entriesCut, p.valuesCut
	return ev
}

type parser struct{ valuesCut, entriesCut bool }

func objectOf(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func listOf(raw json.RawMessage) []json.RawMessage {
	var l []json.RawMessage
	if json.Unmarshal(raw, &l) != nil {
		return nil
	}
	return l
}

// valuesOf reads a bare list or an object holding the list under "values".
func valuesOf(raw json.RawMessage) []json.RawMessage {
	if l := listOf(raw); l != nil {
		return l
	}
	return listOf(objectOf(raw)["values"])
}

// decode reads raw as a T; null and a value of another type are not found.
func decode[T any](raw json.RawMessage) (T, bool) {
	var v *T
	if json.Unmarshal(raw, &v) != nil || v == nil {
		var zero T
		return zero, false
	}
	return *v, true
}

func (p *parser) cut(s string) string {
	if len(s) <= MaxValueLength {
		return s
	}
	n := 0
	for i := range s {
		if n == MaxValueLength {
			p.valuesCut = true
			return s[:i] + "…"
		}
		n++
	}
	return s
}

// str is the text of a JSON string and empty for any other type.
func (p *parser) str(raw json.RawMessage) string {
	s, _ := decode[string](raw)
	return p.cut(s)
}

// text is a string as it is, null as empty, any other value as compact JSON.
func (p *parser) text(raw json.RawMessage) string {
	if s, ok := decode[string](raw); ok {
		return p.cut(s)
	}
	var b bytes.Buffer
	if string(raw) == "null" || json.Compact(&b, raw) != nil {
		return ""
	}
	return p.cut(b.String())
}

func (p *parser) number(raw json.RawMessage) (float64, bool) {
	f, ok := decode[float64](raw)
	return f, ok && !math.IsNaN(f) && !math.IsInf(f, 0)
}

func (p *parser) integer(raw json.RawMessage) *int {
	f, ok := p.number(raw)
	if !ok || f != math.Trunc(f) || math.Abs(f) > 1<<31 {
		return nil
	}
	n := int(f)
	return &n
}

func (p *parser) boolean(raw json.RawMessage) *bool {
	b, ok := decode[bool](raw)
	if !ok {
		return nil
	}
	return &b
}

func (p *parser) message(root map[string]json.RawMessage) string {
	entry := objectOf(root["logentry"])
	for _, raw := range []json.RawMessage{entry["formatted"], entry["message"]} {
		if s := p.str(raw); s != "" {
			return s
		}
	}
	if s := p.str(root["message"]); s != "" {
		return s
	}
	msg := objectOf(root["message"])
	for _, raw := range []json.RawMessage{msg["formatted"], msg["message"]} {
		if s := p.str(raw); s != "" {
			return s
		}
	}
	return ""
}

// pairs reads an object, or a list of [key, value] pairs, and keeps the first MaxEntries by key.
func (p *parser) pairs(raw json.RawMessage) []KeyValue {
	type entry struct {
		key   string
		value json.RawMessage
	}
	var entries []entry
	if m := objectOf(raw); m != nil {
		for k, v := range m {
			entries = append(entries, entry{k, v})
		}
	} else {
		for _, item := range listOf(raw) {
			if kv := listOf(item); len(kv) == 2 {
				k, _ := decode[string](kv[0])
				entries = append(entries, entry{k, kv[1]})
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	if len(entries) > MaxEntries {
		entries, p.entriesCut = entries[:MaxEntries], true
	}
	var out []KeyValue
	for _, e := range entries {
		out = append(out, KeyValue{p.cut(e.key), p.text(e.value)})
	}
	return out
}

func (p *parser) tags(raw json.RawMessage) []KeyValue { return p.pairs(raw) }

func (p *parser) contexts(raw json.RawMessage) []EventContext {
	var names []string
	objects := map[string]json.RawMessage{}
	for name, v := range objectOf(raw) {
		if objectOf(v) != nil {
			names = append(names, name)
			objects[name] = v
		}
	}
	sort.Strings(names)
	if len(names) > MaxEntries {
		names, p.entriesCut = names[:MaxEntries], true
	}
	var out []EventContext
	for _, name := range names {
		out = append(out, EventContext{Name: p.cut(name), Fields: p.pairs(objects[name])})
	}
	return out
}

func (p *parser) exceptions(raw json.RawMessage) ([]Exception, bool) {
	values := valuesOf(raw)
	cut := len(values) > MaxExceptions
	if cut {
		values = values[len(values)-MaxExceptions:]
	}
	var out []Exception
	for _, v := range values {
		m := objectOf(v)
		if m == nil {
			continue
		}
		mech := objectOf(m["mechanism"])
		ex := Exception{
			Type: p.str(m["type"]), Value: p.text(m["value"]), Module: p.str(m["module"]),
			Mechanism: p.str(mech["type"]), Handled: p.boolean(mech["handled"]),
		}
		ex.Frames, ex.FramesOmitted = p.frames(m["stacktrace"])
		out = append(out, ex)
	}
	return out, cut
}

func (p *parser) frames(stacktrace json.RawMessage) ([]Frame, int) {
	raw := listOf(objectOf(stacktrace)["frames"])
	omitted := 0
	if len(raw) > MaxFrames {
		omitted = len(raw) - MaxFrames
		raw = raw[omitted:]
	}
	var out []Frame
	for _, r := range raw {
		if m := objectOf(r); m != nil {
			out = append(out, p.frame(m))
		}
	}
	return out, omitted
}

func (p *parser) frame(m map[string]json.RawMessage) Frame {
	f := Frame{
		Function: p.str(m["function"]), Module: p.str(m["module"]),
		Lineno: p.integer(m["lineno"]), Colno: p.integer(m["colno"]),
		PreContext: p.contextLines(lastN(listOf(m["pre_context"]), MaxContextLines)),
	}
	for _, key := range []string{"filename", "abs_path", "module", "package"} {
		if f.Filename = p.str(m[key]); f.Filename != "" {
			break
		}
	}
	if in := p.boolean(m["in_app"]); in != nil {
		f.InApp = *in
	}
	if line, ok := decode[string](m["context_line"]); ok {
		line = p.cut(line)
		f.ContextLine = &line
	}
	post := listOf(m["post_context"])
	if len(post) > MaxContextLines {
		post = post[:MaxContextLines]
	}
	f.PostContext = p.contextLines(post)
	return f
}

// contextLines reads source lines, keeping an item that is not a string as an empty line.
func (p *parser) contextLines(items []json.RawMessage) []string {
	var out []string
	for _, item := range items {
		s, _ := decode[string](item)
		out = append(out, p.cut(s))
	}
	return out
}

func lastN[T any](s []T, n int) []T {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func (p *parser) breadcrumbs(raw json.RawMessage) ([]Breadcrumb, bool) {
	values := valuesOf(raw)
	cut := len(values) > MaxBreadcrumbs
	if cut {
		values = values[len(values)-MaxBreadcrumbs:]
	}
	var out []Breadcrumb
	for _, v := range values {
		m := objectOf(v)
		if m == nil {
			continue
		}
		out = append(out, Breadcrumb{
			Timestamp: p.timestamp(m["timestamp"]),
			Type:      p.str(m["type"]), Category: p.str(m["category"]), Level: p.str(m["level"]),
			Message: p.str(m["message"]), Data: p.pairs(m["data"]),
		})
	}
	return out, cut
}

// maxEpochSeconds is the last second of year 9999.
const maxEpochSeconds = 253402300799

// timestamp reads an RFC 3339 string or epoch seconds.
func (p *parser) timestamp(raw json.RawMessage) *time.Time {
	if s, ok := decode[string](raw); ok {
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil
		}
		return &t
	}
	f, ok := p.number(raw)
	if !ok || f < 0 || f > maxEpochSeconds {
		return nil
	}
	sec := math.Floor(f)
	t := time.Unix(int64(sec), int64(math.Round((f-sec)*1e9))).UTC()
	return &t
}

func hasFrames(exceptions []Exception) bool {
	for _, ex := range exceptions {
		if len(ex.Frames) > 0 {
			return true
		}
	}
	return false
}

// payloadHasFrames reports frames in the threads or the event-level stacktrace.
func payloadHasFrames(root map[string]json.RawMessage) bool {
	if len(listOf(objectOf(root["stacktrace"])["frames"])) > 0 {
		return true
	}
	for _, t := range valuesOf(root["threads"]) {
		if len(listOf(objectOf(objectOf(t)["stacktrace"])["frames"])) > 0 {
			return true
		}
	}
	return false
}

// cutBytes cuts s to at most max bytes without splitting a character.
func cutBytes(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max], true
}
