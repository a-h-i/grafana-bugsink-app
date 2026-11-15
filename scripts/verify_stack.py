#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Check that an error in the sample app is findable in Bugsink, Tempo and Loki under one trace id."""

import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

BUGSINK = os.environ.get("BUGSINK_URL", "http://localhost:8800").rstrip("/")
GRAFANA = os.environ.get("GRAFANA_URL", "http://localhost:3300").rstrip("/")
SAMPLE = os.environ.get("SAMPLE_PYTHON_URL", "http://localhost:8810").rstrip("/")
SAMPLE_JS = os.environ.get("SAMPLE_JS_URL", "http://localhost:8820").rstrip("/")
# Seeded GLOBAL_READ token: a public, development-only value.
GLOBAL_READ = "a" * 40
SETTLE_S = 10


class RequestFailed(Exception):
    pass


def request(url, method="GET", headers=None):
    """Return the decoded JSON body, or None on 404 (not there yet)."""
    req = urllib.request.Request(url, method=method, headers=headers or {}, data=b"" if method == "POST" else None)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.load(resp)
    except ValueError as err:
        raise RequestFailed(f"{url}: body is not JSON ({err})") from err
    except urllib.error.HTTPError as err:
        if err.code == 404:
            return None
        raise RequestFailed(f"{url}: HTTP {err.code}") from err
    except OSError as err:
        raise RequestFailed(f"{url}: {err}") from err


def bugsink(path):
    return request(f"{BUGSINK}/api/canonical/0/{path}", headers={"Authorization": f"Bearer {GLOBAL_READ}"})


def grafana_proxy(path, **params):
    query = "?" + urllib.parse.urlencode(params) if params else ""
    return request(f"{GRAFANA}/api/datasources/proxy/uid/{path}{query}")


def poll(check, deadline_s=30):
    """Retry check every half second until it returns something truthy or the deadline passes."""
    end = time.monotonic() + deadline_s
    while True:
        value = check()
        if value or time.monotonic() >= end:
            return value
        time.sleep(0.5)


def lookup_event(event_id, project=1):
    return bugsink(f"events/lookup/?project={project}&event_id={event_id}")


def trace_id_of(event):
    return event["data"].get("contexts", {}).get("trace", {}).get("trace_id")


def span_count(trace):
    return sum(len(s["spans"]) for b in trace["batches"] for s in b["scopeSpans"])


def loki_lines(trace_id, service):
    now = time.time_ns()
    result = grafana_proxy(
        "loki/loki/api/v1/query_range",
        query=f'{{service_name="{service}"}} | trace_id="{trace_id}"',
        start=now - 15 * 60 * 10**9,
        end=now,
        limit=100,
    )
    return [v for stream in result["data"]["result"] for v in stream["values"]]


def check_handled(state, sample=SAMPLE, label="sample", incoming=None):
    headers = {"traceparent": f"00-{incoming}-{os.urandom(8).hex()}-01"} if incoming else None
    handled = request(f"{sample}/handled", headers=headers) or {}
    state["trace_id"], state["event_id"] = handled.get("trace_id"), handled.get("event_id")
    message = f"{label} /handled returned trace_id={state['trace_id']} event_id={state['event_id']}"
    if incoming:
        return bool(all(state.values())) and state["trace_id"] == incoming, f"{message}, sent traceparent trace id {incoming}"
    return bool(all(state.values())), message


def check_bugsink_event(state, project=1):
    event = poll(lambda: lookup_event(state["event_id"], project))
    seen = trace_id_of(event) if event else None
    state["issue_id"] = event["issue"] if event else None
    return seen == state["trace_id"], f"bugsink event {state['event_id']} has trace id {seen}"


def check_bugsink_issue(state):
    def found():
        return (bugsink(f"issues/?project=1&q=trace:{state['trace_id']}") or {})["results"]

    ids = [issue["id"] for issue in poll(found) or []]
    expected = state["issue_id"]
    return ids == [expected], f"bugsink search trace:{state['trace_id']} returned {len(ids)} issue(s) {ids}, expected [{expected}]"


def check_tempo(state):
    trace = poll(lambda: grafana_proxy(f"tempo/api/traces/{state['trace_id']}"))
    spans = span_count(trace) if trace else 0
    return spans >= 1, f"tempo trace {state['trace_id']} has {spans} span(s)"


def check_loki(state, service="sample-python"):
    lines = poll(lambda: loki_lines(state["trace_id"], service))
    return bool(lines), f"loki has {len(lines or [])} line(s) with trace_id {state['trace_id']}"


def check_traceparent(state):
    incoming = os.urandom(16).hex()
    headers = {"traceparent": f"00-{incoming}-{os.urandom(8).hex()}-01"}
    seen = (request(f"{SAMPLE}/handled", headers=headers) or {}).get("trace_id")
    return seen == incoming, f"traceparent {incoming} answered as trace id {seen}"


def check_no_trace(state):
    event_id = (request(f"{SAMPLE}/no-trace", method="POST") or {}).get("event_id")
    event = poll(lambda: lookup_event(event_id)) if event_id else None
    if not event:
        return False, f"no-trace event {event_id} not found in bugsink"
    orphan = trace_id_of(event)
    if orphan is None:
        return True, "no-trace event: no trace id in Bugsink"
    found = poll(lambda: grafana_proxy(f"tempo/api/traces/{orphan}"), deadline_s=SETTLE_S)
    where = "present in Tempo" if found else "absent from Tempo"
    return not found, f"no-trace event: trace id {orphan} present in Bugsink, {where}"


# (name, check, name of the check it needs or None, state key)
CHECKS = [
    ("sample /handled", check_handled, None, "python"),
    ("bugsink event", check_bugsink_event, "sample /handled", "python"),
    ("bugsink issue search", check_bugsink_issue, "sample /handled", "python"),
    ("tempo trace", check_tempo, "sample /handled", "python"),
    ("loki logs", check_loki, "sample /handled", "python"),
    ("no-trace event", check_no_trace, None, "python"),
    ("traceparent kept", check_traceparent, None, "python"),
    (
        "sample-js /handled",
        lambda state: check_handled(state, SAMPLE_JS, "sample-js", incoming=os.urandom(16).hex()),
        None,
        "js",
    ),
    ("sample-js bugsink event", lambda state: check_bugsink_event(state, project=2), "sample-js /handled", "js"),
    ("sample-js tempo trace", check_tempo, "sample-js /handled", "js"),
    ("sample-js loki logs", lambda state: check_loki(state, "sample-js"), "sample-js /handled", "js"),
]


def main():
    states = {"python": {}, "js": {}}
    passed = {}
    for name, check, needs, key in CHECKS:
        if needs is not None and not passed[needs]:
            ok, message = False, f"{name}: skipped, {needs} check failed"
        else:
            try:
                ok, message = check(states[key])
            except (RequestFailed, KeyError, TypeError, AttributeError, ValueError) as err:
                ok, message = False, f"{name}: {type(err).__name__}: {err}"
        passed[name] = ok
        print(("ok   " if ok else "FAIL ") + message)
    return 0 if all(passed.values()) else 1


if __name__ == "__main__":
    sys.exit(main())
