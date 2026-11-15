// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"reflect"
	"testing"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type fieldSpec struct {
	name, display string
	typ           data.FieldType
}

var contractFields = []fieldSpec{
	{"friendly_id", "Issue", data.FieldTypeString},
	{"calculated_type", "Type", data.FieldTypeString},
	{"calculated_value", "Message", data.FieldTypeString},
	{"transaction", "Transaction", data.FieldTypeString},
	{"project", "Project", data.FieldTypeString},
	{"status", "Status", data.FieldTypeString},
	{"digested_event_count", "Events", data.FieldTypeInt64},
	{"stored_event_count", "Stored events", data.FieldTypeInt64},
	{"first_seen", "First seen", data.FieldTypeTime},
	{"last_seen", "Last seen", data.FieldTypeTime},
	{"is_resolved", "Resolved", data.FieldTypeBool},
	{"is_resolved_by_next_release", "Resolved by next release", data.FieldTypeBool},
	{"is_muted", "Muted", data.FieldTypeBool},
	{"unmute_after", "Unmute after", data.FieldTypeNullableTime},
	{"project_id", "Project ID", data.FieldTypeInt64},
	{"id", "", data.FieldTypeString},
}

func sampleIssues() []bugsink.Issue {
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	unmute := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	return []bugsink.Issue{
		{ID: "id-1", FriendlyID: "SAMPLE-1", ProjectID: 1, Type: "ValueError", Value: "boom", Transaction: "/a",
			FirstSeen: first, LastSeen: last, DigestedEventCount: 5, StoredEventCount: 4, IsResolved: true, IsResolvedByNextRelease: true},
		{ID: "id-2", FriendlyID: "SAMPLE-2", ProjectID: 2, Type: "KeyError", Value: "k", Transaction: "/b",
			FirstSeen: first, LastSeen: last, DigestedEventCount: 2, StoredEventCount: 2, IsMuted: true, UnmuteAfter: &unmute},
		{ID: "id-3", FriendlyID: "SAMPLE-3", ProjectID: 9, Type: "OSError", Value: "x", Transaction: "/c",
			FirstSeen: first, LastSeen: last, DigestedEventCount: 1, StoredEventCount: 1},
	}
}

func TestIssuesFrame(t *testing.T) {
	frame := issuesFrame(sampleIssues(), map[int]string{1: "alpha", 2: "beta"}, "http://b", false)

	if frame.Name != "issues" || frame.Meta == nil || frame.Meta.PreferredVisualization != data.VisTypeTable {
		t.Fatalf("frame name %q meta %+v", frame.Name, frame.Meta)
	}
	if len(frame.Meta.Notices) != 0 {
		t.Fatalf("unexpected notices %+v", frame.Meta.Notices)
	}
	if len(frame.Fields) != len(contractFields) {
		t.Fatalf("%d fields, want %d", len(frame.Fields), len(contractFields))
	}
	for i, want := range contractFields {
		f := frame.Fields[i]
		if f.Name != want.name || f.Type() != want.typ || f.Config == nil && want.display != "" ||
			f.Config != nil && f.Config.DisplayNameFromDS != want.display {
			t.Errorf("field %d: %q %v %+v, want %+v", i, f.Name, f.Type(), f.Config, want)
		}
		if want.name != "friendly_id" && f.Config != nil && len(f.Config.Links) != 0 {
			t.Errorf("field %q has links", f.Name)
		}
	}

	links := frame.Fields[0].Config.Links
	wantLink := data.DataLink{Title: "Open in Bugsink", URL: "http://b/issues/issue/${__data.fields.id}/event/last/", TargetBlank: true}
	if len(links) != 1 || links[0] != wantLink {
		t.Fatalf("links %+v, want %+v", links, wantLink)
	}

	row := func(i int) []any {
		out := make([]any, len(frame.Fields))
		for j := range out {
			out[j] = frame.Fields[j].At(i)
		}
		return out
	}
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	unmute := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	var noTime *time.Time
	wantRows := [][]any{
		{"SAMPLE-1", "ValueError", "boom", "/a", "alpha", "resolved", int64(5), int64(4), first, last, true, true, false, noTime, int64(1), "id-1"},
		{"SAMPLE-2", "KeyError", "k", "/b", "beta", "muted", int64(2), int64(2), first, last, false, false, true, &unmute, int64(2), "id-2"},
		{"SAMPLE-3", "OSError", "x", "/c", "9", "open", int64(1), int64(1), first, last, false, false, false, noTime, int64(9), "id-3"},
	}
	for i, want := range wantRows {
		if got := row(i); !reflect.DeepEqual(got, want) {
			t.Errorf("row %d:\n got %v\nwant %v", i, got, want)
		}
	}
}

func TestIssuesFrameTruncatedNotice(t *testing.T) {
	frame := issuesFrame(nil, nil, "http://b", true)
	want := data.Notice{Severity: data.NoticeSeverityWarning, Text: "Only the first 5000 issues were read. Narrow the search or the time range."}
	if len(frame.Meta.Notices) != 1 || frame.Meta.Notices[0] != want {
		t.Fatalf("notices %+v, want %+v", frame.Meta.Notices, want)
	}
}

func TestIssuesFrameEmpty(t *testing.T) {
	frame := issuesFrame(nil, nil, "http://b", false)
	if len(frame.Fields) != len(contractFields) {
		t.Fatalf("%d fields, want %d", len(frame.Fields), len(contractFields))
	}
	for _, f := range frame.Fields {
		if f.Len() != 0 {
			t.Errorf("field %q has %d rows", f.Name, f.Len())
		}
	}
}

var eventContractFields = []fieldSpec{
	{"timestamp", "Timestamp", data.FieldTypeTime},
	{"event_id", "Event ID", data.FieldTypeString},
	{"issue", "Issue", data.FieldTypeString},
	{"project", "Project", data.FieldTypeString},
	{"ingested_at", "Ingested at", data.FieldTypeTime},
	{"id", "", data.FieldTypeString},
	{"issue_id", "", data.FieldTypeString},
	{"project_id", "Project ID", data.FieldTypeInt64},
}

func TestEventsFrame(t *testing.T) {
	ts := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	ingested := time.Date(2026, 2, 1, 0, 0, 5, 0, time.UTC)
	rows := []eventRow{
		{event: bugsink.Event{ID: "e-1", EventID: "sdk-1", IssueID: "i-1", ProjectID: 1, Timestamp: ts, IngestedAt: ingested}, issue: "SAMPLE-1"},
		{event: bugsink.Event{ID: "e-2", EventID: "sdk-2", IssueID: "i-2", ProjectID: 9, Timestamp: ts, IngestedAt: ingested}, issue: "SAMPLE-2"},
	}
	frame := eventsFrame(rows, map[int]string{1: "alpha"}, "http://b", []string{"first", "second"})

	if frame.Name != "events" || frame.Meta == nil || frame.Meta.PreferredVisualization != data.VisTypeTable {
		t.Fatalf("frame name %q meta %+v", frame.Name, frame.Meta)
	}
	if len(frame.Fields) != len(eventContractFields) {
		t.Fatalf("%d fields, want %d", len(frame.Fields), len(eventContractFields))
	}
	for i, want := range eventContractFields {
		f := frame.Fields[i]
		if f.Name != want.name || f.Type() != want.typ || f.Config == nil && want.display != "" ||
			f.Config != nil && f.Config.DisplayNameFromDS != want.display {
			t.Errorf("field %d: %q %v %+v, want %+v", i, f.Name, f.Type(), f.Config, want)
		}
		if want.name != "event_id" && f.Config != nil && len(f.Config.Links) != 0 {
			t.Errorf("field %q has links", f.Name)
		}
	}
	wantLink := data.DataLink{
		Title: "Open in Bugsink", URL: "http://b/issues/issue/${__data.fields.issue_id}/event/${__data.fields.id}/", TargetBlank: true,
	}
	if links := frame.Fields[1].Config.Links; len(links) != 1 || links[0] != wantLink {
		t.Fatalf("links %+v, want %+v", links, wantLink)
	}

	want := [][]any{
		{ts, "sdk-1", "SAMPLE-1", "alpha", ingested, "e-1", "i-1", int64(1)},
		{ts, "sdk-2", "SAMPLE-2", "9", ingested, "e-2", "i-2", int64(9)},
	}
	for i, w := range want {
		got := make([]any, len(frame.Fields))
		for j := range got {
			got[j] = frame.Fields[j].At(i)
		}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("row %d:\n got %v\nwant %v", i, got, w)
		}
	}
	wantNotices := []data.Notice{
		{Severity: data.NoticeSeverityWarning, Text: "first"}, {Severity: data.NoticeSeverityWarning, Text: "second"},
	}
	if !reflect.DeepEqual(frame.Meta.Notices, wantNotices) {
		t.Fatalf("notices %+v", frame.Meta.Notices)
	}
}

func TestEventsFrameEmpty(t *testing.T) {
	frame := eventsFrame(nil, nil, "http://b", nil)
	if len(frame.Fields) != len(eventContractFields) {
		t.Fatalf("%d fields, want %d", len(frame.Fields), len(eventContractFields))
	}
	for i, f := range frame.Fields {
		if f.Len() != 0 || f.Type() != eventContractFields[i].typ {
			t.Errorf("field %q has %d rows of %v", f.Name, f.Len(), f.Type())
		}
	}
}
