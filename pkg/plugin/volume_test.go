// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"reflect"
	"testing"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

var volumeNow = utc("2026-11-15T13:48:00Z")

func TestVolumeWindow(t *testing.T) {
	week := volumeNow.Add(-7 * 24 * time.Hour)
	tests := []struct {
		name       string
		from, to   time.Time
		interval   time.Duration
		start, end time.Time
		step       time.Duration
		toNow, cut bool
		empty      bool
		now        time.Time
	}{
		{name: "week, 10m", from: week, to: volumeNow, interval: 10 * time.Minute,
			start: utc("2026-11-08T13:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true},
		{name: "exactly one hour", from: week, to: volumeNow, interval: time.Hour,
			start: utc("2026-11-08T13:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true},
		{name: "90 minutes", from: week, to: volumeNow, interval: 90 * time.Minute,
			start: utc("2026-11-08T12:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: 2 * time.Hour, toNow: true},
		{name: "a day", from: week, to: volumeNow, interval: 24 * time.Hour,
			start: utc("2026-11-08T00:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: 24 * time.Hour, toNow: true},
		{name: "a week", from: week, to: volumeNow, interval: 7 * 24 * time.Hour,
			start: utc("2026-11-05T00:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: 7 * 24 * time.Hour, toNow: true},
		{name: "crosses the limit", from: volumeNow.Add(-180 * 24 * time.Hour), to: volumeNow, interval: 3 * time.Hour,
			start: utc("2026-08-17T15:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: 3 * time.Hour, toNow: true, cut: true},
		{name: "off the step boundary", from: utc("2026-11-10T10:30:00Z"), to: utc("2026-11-10T12:30:00Z"), interval: time.Minute,
			start: utc("2026-11-10T10:00:00Z"), end: utc("2026-11-10T13:00:00Z"), step: time.Hour},
		{name: "reaches into the future", from: volumeNow.Add(-time.Hour), to: volumeNow.Add(time.Hour), interval: time.Hour,
			start: utc("2026-11-15T12:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true},
		{name: "wholly older than the limit", from: volumeNow.Add(-200 * 24 * time.Hour), to: volumeNow.Add(-150 * 24 * time.Hour), interval: time.Hour,
			step: time.Hour, cut: true, empty: true},
		{name: "wholly in the future", from: utc("2026-11-15T14:00:00Z"), to: utc("2026-11-16T14:00:00Z"), interval: time.Hour,
			step: time.Hour, empty: true},
		{name: "reversed", from: volumeNow, to: week, interval: time.Hour, step: time.Hour, empty: true},
		{name: "empty range", from: volumeNow, to: volumeNow, interval: time.Hour, step: time.Hour, empty: true},
		{name: "zero interval", from: week, to: volumeNow, interval: 0,
			start: utc("2026-11-08T13:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true},
		{name: "negative interval", from: week, to: volumeNow, interval: -time.Hour,
			start: utc("2026-11-08T13:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true},
		{name: "from is the oldest hour kept", from: utc("2026-08-17T14:00:00Z"), to: volumeNow, interval: time.Hour,
			start: utc("2026-08-17T14:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true},
		{name: "from one second before the oldest hour kept", from: utc("2026-08-17T13:59:59Z"), to: volumeNow, interval: time.Hour,
			start: utc("2026-08-17T14:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour, toNow: true, cut: true},
		{name: "from and to on the hour", from: utc("2026-11-10T10:00:00Z"), to: utc("2026-11-10T12:00:00Z"), interval: time.Hour,
			start: utc("2026-11-10T10:00:00Z"), end: utc("2026-11-10T12:00:00Z"), step: time.Hour},
		{name: "from one second before the hour", from: utc("2026-11-10T09:59:59Z"), to: utc("2026-11-10T12:00:00Z"), interval: time.Hour,
			start: utc("2026-11-10T09:00:00Z"), end: utc("2026-11-10T12:00:00Z"), step: time.Hour},
		{name: "from one second after the hour", from: utc("2026-11-10T10:00:01Z"), to: utc("2026-11-10T12:00:00Z"), interval: time.Hour,
			start: utc("2026-11-10T10:00:00Z"), end: utc("2026-11-10T12:00:00Z"), step: time.Hour},
		{name: "to one second after the hour", from: utc("2026-11-10T10:00:00Z"), to: utc("2026-11-10T12:00:01Z"), interval: time.Hour,
			start: utc("2026-11-10T10:00:00Z"), end: utc("2026-11-10T13:00:00Z"), step: time.Hour},
		{name: "now exactly on the hour", now: utc("2026-11-15T14:00:00Z"), from: utc("2026-11-08T14:00:00Z"), to: utc("2026-11-15T14:00:00Z"), interval: time.Hour,
			start: utc("2026-11-08T14:00:00Z"), end: utc("2026-11-15T14:00:00Z"), step: time.Hour},
		{name: "step wider than the retention", from: week, to: volumeNow, interval: 4000 * time.Hour,
			step: 4000 * time.Hour, cut: true, empty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := tt.now
			if now.IsZero() {
				now = volumeNow
			}
			w := windowFor(tt.from, tt.to, now, tt.interval)
			if w.empty() != tt.empty {
				t.Errorf("empty %v, want %v (window %+v)", w.empty(), tt.empty, w)
			}
			if w.Step != tt.step {
				t.Errorf("step %v, want %v", w.Step, tt.step)
			}
			if w.Cut != tt.cut {
				t.Errorf("cut %v, want %v", w.Cut, tt.cut)
			}
			if tt.empty {
				if w.ToNow {
					t.Error("an empty window reaches now")
				}
				return
			}
			if !tt.start.Equal(w.Start) || !tt.end.Equal(w.End) {
				t.Errorf("window %v to %v, want %v to %v", w.Start, w.End, tt.start, tt.end)
			}
			if w.ToNow != tt.toNow {
				t.Errorf("to now %v, want %v", w.ToNow, tt.toNow)
			}
			if w.Start.Location() != time.UTC || w.End.Location() != time.UTC {
				t.Errorf("window is not in UTC: %v to %v", w.Start, w.End)
			}
			if w.End.Sub(w.Start) > maxVolumeHours*time.Hour {
				t.Errorf("window %v is over the bound", w.End.Sub(w.Start))
			}
			var counts []bugsink.EventCount
			var want int64
			for h, i := w.Start, 1; h.Before(w.End); h, i = h.Add(time.Hour), i+1 {
				counts = append(counts, bugsink.EventCount{Hour: h, Count: i})
				want += int64(i)
			}
			_, totals := rebucket(counts, w.Step)
			var got int64
			for _, v := range totals {
				got += v
			}
			if got != want {
				t.Errorf("rebucketed sum %d, want %d", got, want)
			}
		})
	}
}

func TestVolumeWindowAlignsStepsToTheEpoch(t *testing.T) {
	w := windowFor(volumeNow.Add(-30*24*time.Hour), volumeNow, volumeNow, 5*time.Hour)
	if w.Step != 5*time.Hour || w.Start.Unix()%18000 != 0 {
		t.Errorf("step %v starts at %v", w.Step, w.Start)
	}
}

func TestVolumeWindowNeverExceedsTheBound(t *testing.T) {
	ranges := []struct{ from, to time.Time }{
		{time.Time{}, volumeNow},
		{time.Time{}, time.Date(9000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{volumeNow.Add(-91 * 24 * time.Hour), volumeNow.Add(24 * time.Hour)},
	}
	for _, r := range ranges {
		for _, interval := range []time.Duration{-1, 0, time.Second, time.Hour, 7 * time.Hour, 1000 * time.Hour, 1<<63 - 1} {
			w := windowFor(r.from, r.to, volumeNow, interval)
			if w.Step < time.Hour {
				t.Errorf("interval %v: step %v is under an hour", interval, w.Step)
			}
			if !w.empty() && w.End.Sub(w.Start) > maxVolumeHours*time.Hour {
				t.Errorf("interval %v: window %v is over the bound", interval, w.End.Sub(w.Start))
			}
		}
	}
}

func hourly(start string, counts ...int) []bugsink.EventCount {
	t0 := utc(start)
	out := make([]bugsink.EventCount, len(counts))
	for i, c := range counts {
		out[i] = bugsink.EventCount{Hour: t0.Add(time.Duration(i) * time.Hour), Count: c}
	}
	return out
}

func TestRebucket(t *testing.T) {
	counts := hourly("2026-11-10T00:00:00Z", 1, 2, 3, 4, 5, 6)

	starts, totals := rebucket(counts, 3*time.Hour)
	if want := []time.Time{utc("2026-11-10T00:00:00Z"), utc("2026-11-10T03:00:00Z")}; !reflect.DeepEqual(starts, want) {
		t.Errorf("starts %v, want %v", starts, want)
	}
	if want := []int64{6, 15}; !reflect.DeepEqual(totals, want) {
		t.Errorf("totals %v, want %v", totals, want)
	}

	starts, totals = rebucket(counts, time.Hour)
	if len(starts) != 6 || !reflect.DeepEqual(totals, []int64{1, 2, 3, 4, 5, 6}) {
		t.Errorf("hourly: starts %v totals %v", starts, totals)
	}

	starts, totals = rebucket(nil, time.Hour)
	if len(starts) != 0 || len(totals) != 0 {
		t.Errorf("no counts: starts %v totals %v", starts, totals)
	}
}

func TestRebucketToleratesEdgeHours(t *testing.T) {
	starts, totals := rebucket(hourly("2026-11-10T00:00:00Z", 1, 1, 1, 1, 7), 2*time.Hour)
	if want := []time.Time{utc("2026-11-10T00:00:00Z"), utc("2026-11-10T02:00:00Z"), utc("2026-11-10T04:00:00Z")}; !reflect.DeepEqual(starts, want) {
		t.Errorf("starts %v, want %v", starts, want)
	}
	if want := []int64{2, 2, 7}; !reflect.DeepEqual(totals, want) {
		t.Errorf("totals %v, want %v", totals, want)
	}

	starts, totals = rebucket(hourly("2026-11-10T00:00:00Z", 1, 1, 1), 2*time.Hour)
	if want := []time.Time{utc("2026-11-10T00:00:00Z"), utc("2026-11-10T02:00:00Z")}; !reflect.DeepEqual(starts, want) {
		t.Errorf("starts %v, want %v", starts, want)
	}
	if want := []int64{2, 1}; !reflect.DeepEqual(totals, want) {
		t.Errorf("totals %v, want %v", totals, want)
	}
}

func TestRebucketKeepsTheSumOfTheWindow(t *testing.T) {
	w := windowFor(utc("2026-11-09T05:20:00Z"), utc("2026-11-12T22:10:00Z"), volumeNow, 7*time.Hour)
	if w.empty() {
		t.Fatal("empty window")
	}
	var counts []bugsink.EventCount
	var want int64
	for h, i := w.Start, 1; h.Before(w.End); h, i = h.Add(time.Hour), i+1 {
		counts = append(counts, bugsink.EventCount{Hour: h, Count: i})
		want += int64(i)
	}
	starts, totals := rebucket(counts, w.Step)
	var got int64
	for _, v := range totals {
		got += v
	}
	if got != want {
		t.Errorf("sum %d, want %d", got, want)
	}
	if !starts[0].Equal(w.Start) {
		t.Errorf("first start %v, want %v", starts[0], w.Start)
	}
	if len(starts) > maxVolumeHours {
		t.Errorf("%d points", len(starts))
	}
}

func TestStepNotice(t *testing.T) {
	n, ok := stepNotice(time.Hour, 10*time.Minute)
	if want := (data.Notice{Severity: data.NoticeSeverityInfo, Text: "Each point counts one hour of events: Bugsink counts events per hour."}); !ok || n != want {
		t.Errorf("one hour: %v %v", n, ok)
	}

	n, ok = stepNotice(3*time.Hour, 150*time.Minute)
	if want := (data.Notice{Severity: data.NoticeSeverityInfo, Text: "Each point counts 3 hours of events: Bugsink counts events per hour."}); !ok || n != want {
		t.Errorf("three hours: %v %v", n, ok)
	}

	if _, ok = stepNotice(3*time.Hour, 3*time.Hour); ok {
		t.Error("a notice for an interval that is already whole hours")
	}
}

func TestVolumeFrame(t *testing.T) {
	starts := []time.Time{utc("2026-11-10T00:00:00Z"), utc("2026-11-10T01:00:00Z")}
	notice := data.Notice{Severity: data.NoticeSeverityWarning, Text: "careful"}
	f := volumeFrame("sample-python", data.Labels{"project": "sample-python"}, time.Hour, starts, []int64{4, 5}, []data.Notice{notice})

	if f.Name != "sample-python" || len(f.Fields) != 2 {
		t.Fatalf("frame %q with %d fields", f.Name, len(f.Fields))
	}
	if f.Fields[0].Name != "time" || f.Fields[1].Name != "count" {
		t.Errorf("fields %q, %q", f.Fields[0].Name, f.Fields[1].Name)
	}
	if f.Fields[0].Type() != data.FieldTypeTime || f.Fields[1].Type() != data.FieldTypeInt64 {
		t.Errorf("types %v, %v", f.Fields[0].Type(), f.Fields[1].Type())
	}
	if f.Fields[0].Config.Interval != float64(3600000) {
		t.Errorf("interval %v", f.Fields[0].Config.Interval)
	}
	if !reflect.DeepEqual(f.Fields[1].Labels, data.Labels{"project": "sample-python"}) || f.Fields[1].Config.DisplayNameFromDS != "sample-python" {
		t.Errorf("labels %v, display name %q", f.Fields[1].Labels, f.Fields[1].Config.DisplayNameFromDS)
	}
	if f.Meta.Type != data.FrameTypeTimeSeriesMulti || f.Meta.TypeVersion != (data.FrameTypeVersion{0, 1}) || f.Meta.PreferredVisualization != data.VisTypeGraph {
		t.Errorf("meta %+v", f.Meta)
	}
	if !reflect.DeepEqual(f.Meta.Notices, []data.Notice{notice}) {
		t.Errorf("notices %v", f.Meta.Notices)
	}
	if !reflect.DeepEqual(timesOf(f.Fields[0]), starts) || f.Fields[1].At(1) != int64(5) {
		t.Errorf("times %v, second count %v", timesOf(f.Fields[0]), f.Fields[1].At(1))
	}

	empty := volumeFrame("x", nil, time.Hour, nil, nil, nil)
	if len(empty.Fields) != 2 || empty.Fields[0].Len() != 0 || empty.Fields[1].Len() != 0 {
		t.Fatalf("empty frame %+v", empty.Fields)
	}
	if empty.Fields[0].Type() != data.FieldTypeTime || empty.Fields[1].Type() != data.FieldTypeInt64 {
		t.Errorf("empty types %v, %v", empty.Fields[0].Type(), empty.Fields[1].Type())
	}
}

func timesOf(f *data.Field) []time.Time {
	out := make([]time.Time, f.Len())
	for i := range out {
		out[i] = f.At(i).(time.Time)
	}
	return out
}
