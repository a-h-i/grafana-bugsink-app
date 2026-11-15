// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	queryTypeVolume      = "volume"
	maxVolumeHours       = bugsink.MaxEventCountBuckets
	countCacheTTL        = time.Minute
	countCacheMaxEntries = 128

	volumeNamesNothing = "An event volume query needs a project or an issue."
	volumeCutNotice    = "Bugsink keeps hourly event counts for 90 days, so the series starts later than the time range."
)

// volumeWindow is the hours one volume query reads from Bugsink.
type volumeWindow struct {
	Start, End time.Time     // UTC hours, Start inclusive, End exclusive
	Step       time.Duration // whole hours, at least one
	ToNow      bool          // End is the end of the current hour, so no end parameter is sent
	Cut        bool          // the 90-day limit moved Start after the range start
}

func (w volumeWindow) empty() bool { return !w.End.After(w.Start) }

// windowFor aligns the range to steps of whole hours and clamps it to the hours Bugsink keeps.
func windowFor(from, to, now time.Time, interval time.Duration) volumeWindow {
	step := stepFor(interval)
	w := volumeWindow{Step: step}
	if !to.After(from) {
		return w
	}
	stepSeconds := int64(step / time.Second)
	endOfNow := now.UTC().Truncate(time.Hour).Add(time.Hour)
	oldest := endOfNow.Add(-maxVolumeHours * time.Hour)

	if !to.After(oldest) {
		w.Cut = true
		return w
	}
	if !from.Before(endOfNow) {
		return w
	}

	firstStep := alignUp(oldest, stepSeconds)
	start := alignDown(from, stepSeconds)
	if start.Before(firstStep) {
		start = firstStep
		w.Cut = true
	}
	end := endOfNow
	if aligned := alignUp(minTime(to, endOfNow), stepSeconds); aligned.Before(endOfNow) {
		end = aligned
	} else {
		w.ToNow = true
	}
	if !end.After(start) {
		w.ToNow = false
		return w
	}
	w.Start, w.End = start, end
	return w
}

// maxStep keeps the rounding of a huge interval from overflowing.
const maxStep = 1 << 20 * time.Hour

// stepFor rounds interval up to whole hours, at least one.
func stepFor(interval time.Duration) time.Duration {
	if interval <= time.Hour {
		return time.Hour
	}
	if interval >= maxStep {
		return maxStep
	}
	hours := interval / time.Hour
	if interval%time.Hour > 0 {
		hours++
	}
	return hours * time.Hour
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func alignDown(t time.Time, stepSeconds int64) time.Time {
	s := t.Unix()
	r := s % stepSeconds
	if r < 0 {
		r += stepSeconds
	}
	return time.Unix(s-r, 0).UTC()
}

func alignUp(t time.Time, stepSeconds int64) time.Time {
	down := alignDown(t, stepSeconds)
	if down.Equal(t) {
		return down
	}
	return down.Add(time.Duration(stepSeconds) * time.Second)
}

// rebucket sums ascending hourly counts into steps aligned to the epoch.
func rebucket(counts []bugsink.EventCount, step time.Duration) (starts []time.Time, totals []int64) {
	stepSeconds := int64(step / time.Second)
	for _, c := range counts {
		start := alignDown(c.Hour, stepSeconds)
		if n := len(starts); n > 0 && starts[n-1].Equal(start) {
			totals[n-1] += int64(c.Count)
			continue
		}
		starts = append(starts, start)
		totals = append(totals, int64(c.Count))
	}
	return starts, totals
}

// stepNotice says each point spans more than the dashboard interval asked for.
func stepNotice(step, interval time.Duration) (data.Notice, bool) {
	if step == interval {
		return data.Notice{}, false
	}
	span := "one hour"
	if hours := int64(step / time.Hour); hours > 1 {
		span = fmt.Sprintf("%d hours", hours)
	}
	return data.Notice{
		Severity: data.NoticeSeverityInfo,
		Text:     fmt.Sprintf("Each point counts %s of events: Bugsink counts events per hour.", span),
	}, true
}

// volumeFrame builds the time-series frame of the volume frame contract.
func volumeFrame(name string, labels data.Labels, step time.Duration, starts []time.Time, totals []int64, notices []data.Notice) *data.Frame {
	timeField := data.NewField("time", nil, orEmpty(starts))
	timeField.Config = &data.FieldConfig{Interval: float64(step.Milliseconds())}
	countField := data.NewField("count", labels, orEmpty(totals))
	countField.Config = &data.FieldConfig{DisplayNameFromDS: name}
	frame := data.NewFrame(name, timeField, countField)
	frame.Meta = &data.FrameMeta{
		Type:                   data.FrameTypeTimeSeriesMulti,
		TypeVersion:            data.FrameTypeVersion{0, 1},
		PreferredVisualization: data.VisTypeGraph,
	}
	frame.AppendNotices(notices...)
	return frame
}

// volumeQuery is a validated volume query; Issue wins over ProjectID.
type volumeQuery struct {
	ProjectID int
	Issue     string
}

// parseVolumeQuery validates the volume fields of the query; invalid is the message for a bad query, or empty.
func parseVolumeQuery(m queryModel, project int) (q volumeQuery, invalid string) {
	if m.Issue != "" && !validIssueID(m.Issue) {
		return q, fmt.Sprintf("%q is not an issue id.", m.Issue)
	}
	return volumeQuery{ProjectID: project, Issue: m.Issue}, ""
}

func (q volumeQuery) namesNothing() bool { return q.Issue == "" && q.ProjectID == 0 }

// seriesName is the frame name and the label key and value of the series.
func (d *Datasource) seriesName(ctx context.Context, q volumeQuery) (key, name string) {
	if q.Issue != "" {
		return "issue", q.Issue
	}
	if name, ok := d.projectNames(ctx)[q.ProjectID]; ok {
		return "project", name
	}
	return "project", strconv.Itoa(q.ProjectID)
}

// queryVolume answers the event volume of the project or issue the query names over the time range.
func (d *Datasource) queryVolume(ctx context.Context, q backend.DataQuery, m queryModel, project int) backend.DataResponse {
	vq, invalid := parseVolumeQuery(m, project)
	if invalid != "" {
		return failure(backend.StatusBadRequest, invalid)
	}
	interval := q.Interval
	if vq.namesNothing() {
		notice := data.Notice{Severity: data.NoticeSeverityInfo, Text: volumeNamesNothing}
		return backend.DataResponse{Frames: data.Frames{volumeFrame("", nil, stepFor(interval), nil, nil, []data.Notice{notice})}}
	}
	w := windowFor(q.TimeRange.From, q.TimeRange.To, d.now(), interval)
	var notices []data.Notice
	if w.Cut {
		notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: volumeCutNotice})
	}
	key, name := d.seriesName(ctx, vq)
	labels := data.Labels{key: name}
	if w.empty() {
		return backend.DataResponse{Frames: data.Frames{volumeFrame(name, labels, w.Step, nil, nil, notices)}}
	}
	counts, err := d.eventCounts(ctx, vq, w)
	if err != nil {
		message, status := userMessage(err)
		return failure(status, message)
	}
	if n, ok := stepNotice(w.Step, interval); ok {
		notices = append(notices, n)
	}
	starts, totals := rebucket(counts, w.Step)
	return backend.DataResponse{Frames: data.Frames{volumeFrame(name, labels, w.Step, starts, totals, notices)}}
}

// eventCounts reads the window's hourly counts through the count cache; a failed read is not stored.
func (d *Datasource) eventCounts(ctx context.Context, q volumeQuery, w volumeWindow) ([]bugsink.EventCount, error) {
	kind, id := "project", strconv.Itoa(q.ProjectID)
	if q.Issue != "" {
		kind, id = "issue", q.Issue
	}
	key := fmt.Sprintf("%s|%s|%d|%d", kind, id, w.Start.Unix(), w.End.Unix())
	if cached, ok := d.countCache.get(key); ok {
		return cached, nil
	}
	end := w.End
	if w.ToNow {
		end = time.Time{}
	}
	var (
		counts []bugsink.EventCount
		err    error
	)
	if q.Issue != "" {
		counts, err = d.client.IssueEventCounts(ctx, q.Issue, w.Start, end)
	} else {
		counts, err = d.client.ProjectEventCounts(ctx, q.ProjectID, w.Start, end)
	}
	if err != nil {
		return nil, err
	}
	d.countCache.set(key, counts)
	return counts, nil
}
