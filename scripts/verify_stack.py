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


def lookup_event(event_id):
    return bugsink(f"events/lookup/?project=1&event_id={event_id}")


def trace_id_of(event):
    return event["data"].get("contexts", {}).get("trace", {}).get("trace_id")


def span_count(trace):
    return sum(len(s["spans"]) for b in trace["batches"] for s in b["scopeSpans"])


def loki_lines(trace_id):
    now = time.time_ns()
    result = grafana_proxy(
        "loki/loki/api/v1/query_range",
        query=f'{{service_name="sample-python"}} | trace_id="{trace_id}"',
        start=now - 15 * 60 * 10**9,
        end=now,
        limit=100,
    )
    return [v for stream in result["data"]["result"] for v in stream["values"]]


def check_handled(state):
    handled = request(f"{SAMPLE}/handled") or {}
    state["trace_id"], state["event_id"] = handled.get("trace_id"), handled.get("event_id")
    return bool(all(state.values())), f"sample /handled returned trace_id={state['trace_id']} event_id={state['event_id']}"


def check_bugsink_event(state):
    event = poll(lambda: lookup_event(state["event_id"]))
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


def check_loki(state):
    lines = poll(lambda: loki_lines(state["trace_id"]))
    return bool(lines), f"loki has {len(lines or [])} line(s) with trace_id {state['trace_id']}"


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


# (name, check, needs a successful check 1)
CHECKS = [
    ("sample /handled", check_handled, False),
    ("bugsink event", check_bugsink_event, True),
    ("bugsink issue search", check_bugsink_issue, True),
    ("tempo trace", check_tempo, True),
    ("loki logs", check_loki, True),
    ("no-trace event", check_no_trace, False),
]


def main():
    state = {}
    passed = []
    for name, check, needs_handled in CHECKS:
        if needs_handled and not passed[0]:
            ok, message = False, f"{name}: skipped, /handled check failed"
        else:
            try:
                ok, message = check(state)
            except (RequestFailed, KeyError, TypeError, AttributeError, ValueError) as err:
                ok, message = False, f"{name}: {type(err).__name__}: {err}"
        passed.append(ok)
        print(("ok   " if ok else "FAIL ") + message)
    return 0 if all(passed) else 1


if __name__ == "__main__":
    sys.exit(main())
