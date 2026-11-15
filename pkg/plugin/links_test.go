// SPDX-License-Identifier: GPL-3.0-or-later

package plugin

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/a-h-i/grafana-bugsink-app/pkg/bugsink"
)

func TestParseLinkSettings(t *testing.T) {
	s, invalid := parseLinkSettings(json.RawMessage(`{}`))
	want := linkSettings{window: 15 * time.Minute, logsQuery: defaultLogsQuery, serviceTag: "service"}
	if invalid != "" || s != want {
		t.Fatalf("defaults = %+v %q", s, invalid)
	}
	if s, invalid = parseLinkSettings(nil); invalid != "" || s != want {
		t.Fatalf("nil = %+v %q", s, invalid)
	}

	s, invalid = parseLinkSettings(json.RawMessage(
		`{"tempoDatasourceUid":"t","lokiDatasourceUid":"l","linkWindowMinutes":30,"logsQuery":"{a=\"${service}\"}","serviceTag":"svc"}`))
	want = linkSettings{tempoUID: "t", lokiUID: "l", window: 30 * time.Minute, logsQuery: `{a="${service}"}`, serviceTag: "svc"}
	if invalid != "" || s != want {
		t.Fatalf("all keys = %+v %q", s, invalid)
	}

	if s, invalid = parseLinkSettings(json.RawMessage(`{"serviceTag":""}`)); invalid != "" || s.serviceTag != "" {
		t.Fatalf("empty tag = %+v %q", s, invalid)
	}
	if s, invalid = parseLinkSettings(json.RawMessage(`{"linkWindowMinutes":null}`)); invalid != "" || s.window != 15*time.Minute {
		t.Fatalf("null window = %+v %q", s, invalid)
	}
	if s, invalid = parseLinkSettings(json.RawMessage(`{"linkWindowMinutes":1440}`)); invalid != "" || s.window != 24*time.Hour {
		t.Fatalf("max window = %+v %q", s, invalid)
	}
	if _, invalid = parseLinkSettings(json.RawMessage(`{"tempoDatasourceUid":5}`)); invalid != msgSettingsInvalid {
		t.Fatalf("numeric uid: invalid = %q", invalid)
	}
	if s, invalid = parseLinkSettings(json.RawMessage(`{"logsQuery":"  "}`)); invalid != "" || s.logsQuery != defaultLogsQuery {
		t.Fatalf("blank query = %+v %q", s, invalid)
	}
	for _, q := range []string{defaultLogsQuery, `"a ${service} b"`, `"\"${service}"`, `${foo}`, `{a="${foo}"} | x=` + "`${foo}`"} {
		if _, invalid = parseLinkSettings(json.RawMessage(`{"logsQuery":` + strconv.Quote(q) + `}`)); invalid != "" {
			t.Fatalf("query %s: invalid = %q", q, invalid)
		}
	}
	for _, q := range []string{"{s=`${service}`}", "| status >= ${tag:min}", `"${service}`, "`\"${service}\"`", `{s="${trace_id}"} | x=${release}`, `"\${service}"`, `"${foo"} | x=${service} | y="z"`, "{a=\"b\"} # \"\n | x=${service} # \"", "{a=\"b\"} # ${service}"} {
		if _, invalid = parseLinkSettings(json.RawMessage(`{"logsQuery":` + strconv.Quote(q) + `}`)); invalid != msgLogsQueryQuotes {
			t.Fatalf("query %s: invalid = %q", q, invalid)
		}
	}
	for _, w := range []string{"0", "1441", "2.5", `"15"`, "-3"} {
		if _, invalid = parseLinkSettings(json.RawMessage(`{"linkWindowMinutes":` + w + `}`)); invalid != msgLinkWindow {
			t.Fatalf("window %s: invalid = %q", w, invalid)
		}
	}
}

func TestRenderLogsQuery(t *testing.T) {
	v := logsVars{TraceID: "abc", Service: "api"}
	if got := renderLogsQuery(defaultLogsQuery, v); got != `{service_name="api"} | trace_id="abc"` {
		t.Fatalf("default = %s", got)
	}

	full := logsVars{
		TraceID: "t", Service: "s", Project: "p", Environment: "e", Release: "r",
		Tags: []bugsink.KeyValue{{Key: "region", Value: "eu"}, {Key: "region", Value: "us"}},
	}
	got := renderLogsQuery("${trace_id}|${service}|${project}|${environment}|${release}|${tag:region}|${foo}", full)
	if got != "t|s|p|e|r|eu|${foo}" {
		t.Fatalf("all = %s", got)
	}
	if got = renderLogsQuery("${environment}|${release}|${tag:missing}|${tag:}", logsVars{}); got != "|||" {
		t.Fatalf("missing = %q", got)
	}

	hostile := "a\"} |= \"x\\"
	if got = renderLogsQuery("{s=\"${service}\"}", logsVars{Service: hostile}); got != `{s="a\"} |= \"x\\"}` {
		t.Fatalf("service = %s", got)
	}
	nasty := "q\"b\\k`n\nend"
	want := `q\"b\\k` + "`" + `n\nend`
	for _, v := range []logsVars{
		{TraceID: nasty}, {Service: nasty}, {Project: nasty}, {Environment: nasty}, {Release: nasty},
		{Tags: []bugsink.KeyValue{{Key: "k", Value: nasty}}},
	} {
		got = renderLogsQuery("${trace_id}${service}${project}${environment}${release}${tag:k}", v)
		if got != want {
			t.Fatalf("escaped = %q, want %q", got, want)
		}
	}
	for _, value := range []string{"${__user.login}", "[[x]]"} {
		got = renderLogsQuery(`"${service}"`, logsVars{Service: value})
		if strings.ContainsAny(got, "$[") {
			t.Fatalf("value %s rendered %s", value, got)
		}
	}
	if got = renderLogsQuery(`"${service}"`, logsVars{Service: "${__user.login}"}); got != `"\u0024{__user.login}"` {
		t.Fatalf("dollar = %s", got)
	}
	if got = renderLogsQuery("${trace_id}", logsVars{Project: "${trace_id}", TraceID: "t"}); got != "t" {
		t.Fatalf("trace = %s", got)
	}
	if got = renderLogsQuery("${project}", logsVars{Project: "${trace_id}", TraceID: "t"}); got != `\u0024{trace_id}` {
		t.Fatalf("project = %s", got)
	}
}

func TestLinkVars(t *testing.T) {
	e := &bugsink.EventDetail{
		Environment: "dev", Release: "r1",
		Tags: []bugsink.KeyValue{{Key: "service", Value: "api"}, {Key: "region", Value: "eu"}},
	}
	s := linkSettings{serviceTag: "service"}
	v := s.vars("t", "proj", e)
	if v.Service != "api" || v.Project != "proj" || v.TraceID != "t" || v.Environment != "dev" || v.Release != "r1" || !reflect.DeepEqual(v.Tags, e.Tags) {
		t.Fatalf("vars = %+v", v)
	}
	if v = s.vars("t", "proj", &bugsink.EventDetail{}); v.Service != "proj" {
		t.Fatalf("no tag: %+v", v)
	}
	if v = (linkSettings{}).vars("t", "proj", e); v.Service != "proj" {
		t.Fatalf("empty serviceTag: %+v", v)
	}
	empty := &bugsink.EventDetail{Tags: []bugsink.KeyValue{{Key: "service", Value: ""}}}
	if v = s.vars("t", "proj", empty); v.Service != "proj" {
		t.Fatalf("empty tag value: %+v", v)
	}
	v = s.vars("t", "proj", nil)
	if v.Service != "proj" || v.TraceID != "t" || v.Project != "proj" || v.Environment != "" || v.Release != "" || len(v.Tags) != 0 {
		t.Fatalf("nil detail: %+v", v)
	}
}

func TestExploreLinks(t *testing.T) {
	from := time.Date(2026, 11, 15, 9, 45, 0, 0, time.UTC)
	to := from.Add(35 * time.Minute)
	s := linkSettings{tempoUID: "tempo", lokiUID: "loki"}
	links := s.exploreLinks("${__value.raw}", "{x}", from, to)
	if len(links) != 2 || links[0].Title != "Open trace" || links[1].Title != "Open logs" {
		t.Fatalf("links = %+v", links)
	}
	for i, l := range links {
		if l.Internal == nil || l.Internal.Range == nil || !l.Internal.Range.From.Equal(from) || !l.Internal.Range.To.Equal(to) {
			t.Fatalf("link %d internal = %+v", i, l.Internal)
		}
	}
	if links[0].Internal.DatasourceUID != "tempo" || links[1].Internal.DatasourceUID != "loki" {
		t.Fatalf("uids = %q %q", links[0].Internal.DatasourceUID, links[1].Internal.DatasourceUID)
	}
	want0 := map[string]any{"refId": "A", "queryType": "traceId", "query": "${__value.raw}"}
	want1 := map[string]any{"refId": "A", "queryType": "range", "expr": "{x}"}
	if !reflect.DeepEqual(links[0].Internal.Query, want0) || !reflect.DeepEqual(links[1].Internal.Query, want1) {
		t.Fatalf("queries = %#v %#v", links[0].Internal.Query, links[1].Internal.Query)
	}

	one := linkSettings{lokiUID: "loki"}.exploreLinks("a", "b", from, to)
	if len(one) != 1 || one[0].Title != "Open logs" {
		t.Fatalf("loki only = %+v", one)
	}
	one = linkSettings{tempoUID: "tempo"}.exploreLinks("a", "b", from, to)
	if len(one) != 1 || one[0].Title != "Open trace" {
		t.Fatalf("tempo only = %+v", one)
	}
	if none := (linkSettings{}).exploreLinks("a", "b", from, to); len(none) != 0 {
		t.Fatalf("none = %+v", none)
	}
}
