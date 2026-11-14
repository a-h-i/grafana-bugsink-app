#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Probe Bugsink's REST API with the seeded tokens and print a Markdown report of what it answers."""

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timedelta, timezone

BUGSINK = os.environ.get("BUGSINK_URL", "http://localhost:8800").rstrip("/")
SAMPLE = os.environ.get("SAMPLE_PYTHON_URL", "http://localhost:8810").rstrip("/")
API = f"{BUGSINK}/api/canonical/0/"

# The seeded tokens are public, development-only values.
GLOBAL_READ = "a" * 40
GLOBAL_TRIAGE = "b" * 40
PROJECT_READ = "c" * 40
NO_CAPS = "d" * 40
TOKENS = {"GLOBAL_READ": GLOBAL_READ, "GLOBAL_TRIAGE": GLOBAL_TRIAGE, "PROJECT_READ": PROJECT_READ,
          "NO_CAPS": NO_CAPS, "(no token)": None}

VARIANTS = 130  # enough issues to exceed one page
MUTE_VARIANT = 60  # the /handled variant whose issue the matrix mutes and unmutes
DEADLINE_S = 60
PROJECT2_KEY = "2" * 32  # seeded sentry_key of project 2 (public, development-only)
BODY_LIMIT = 300

# Rows of the matrix: (label, path template). {issue} and {event} are filled from the produced data.
PROBES = [
    ("GET projects/", "projects/"),
    ("GET projects/1/", "projects/1/"),
    ("GET projects/2/", "projects/2/"),
    ("GET issues/?project=1", "issues/?project=1"),
    ("GET issues/?project=2", "issues/?project=2"),
    ("GET issues/ (no project)", "issues/"),
    ("GET issues/?project=1&sort=digest_order", "issues/?project=1&sort=digest_order"),
    ("GET issues/?sort=digest_order (no project)", "issues/?sort=digest_order"),
    ("GET issues/<issue of project 1>/", "issues/{issue}/"),
    ("GET events/?issue=<issue>", "events/?issue={issue}"),
    ("GET events/lookup/?project=1&event_id=<event>", "events/lookup/?project=1&event_id={event}"),
    ("GET issues/<issue>/tags/", "issues/{issue}/tags/"),
    ("GET issue-history/?issue=<issue>", "issue-history/?issue={issue}"),
    ("GET issues/<issue>/event-counts/", "issues/{issue}/event-counts/"),
    ("GET projects/1/event-counts/", "projects/1/event-counts/"),
]


class Unreachable(Exception):
    pass


def call(token, method, path, body=None, base=API):
    """Return (status, parsed JSON or text) for any HTTP response; raise Unreachable only on a network failure."""
    headers = {"Authorization": f"Bearer {token}"} if token else {}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    elif method == "POST":
        data = b""
    req = urllib.request.Request(base + path, method=method, headers=headers, data=data)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            status, raw = resp.status, resp.read()
    except urllib.error.HTTPError as err:
        status, raw = err.code, err.read()
    except OSError as err:
        raise Unreachable(f"{base + path}: {err}") from err
    text = raw.decode("utf-8", "replace")
    try:
        return status, json.loads(text)
    except ValueError:
        return status, text


def sample(method, path):
    return call(None, method, path, base=SAMPLE)


def short(value, limit=BODY_LIMIT):
    text = value if isinstance(value, str) else json.dumps(value, sort_keys=True)
    text = text.replace("\n", " ")
    return text if len(text) <= limit else text[:limit] + "...(truncated)"


def code(value):
    return f"`{short(value).replace('|', '/')}`"


def table(header, rows):
    lines = ["| " + " | ".join(header) + " |", "|" + " --- |" * len(header)]
    lines += ["| " + " | ".join(str(c) for c in row) + " |" for row in rows]
    return "\n".join(lines)


def poll(fn, what, deadline_s=DEADLINE_S):
    """Call fn every 0.5 s until it returns something other than None."""
    end = time.monotonic() + deadline_s
    while time.monotonic() < end:
        result = fn()
        if result is not None:
            return result
        time.sleep(0.5)
    raise RuntimeError(f"timed out after {deadline_s}s waiting for {what}")


def all_pages(token, path, max_pages=50):
    """Follow `next` and return the list of results per page."""
    pages, url = [], API + path
    while url and len(pages) < max_pages:
        status, body = call(token, "GET", url[len(API):])
        if status != 200 or not isinstance(body, dict):
            raise RuntimeError(f"{url}: HTTP {status}")
        pages.append(body["results"])
        url = body.get("next")
    return pages


def lookup_event(event_id):
    status, body = call(GLOBAL_READ, "GET", f"events/lookup/?project=1&event_id={event_id}")
    return body if status == 200 else None


def send_project2_event():
    """Ingest one fixed event into project 2 (the sample app only feeds project 1); return nothing."""
    req = urllib.request.Request(
        f"{BUGSINK}/api/2/store/", method="POST",
        headers={"X-Sentry-Auth": f"Sentry sentry_key={PROJECT2_KEY}, sentry_version=7",
                 "Content-Type": "application/json"},
        data=json.dumps({"message": "probe event for project 2", "level": "error"}).encode())
    try:
        urllib.request.urlopen(req, timeout=15).read()
    except urllib.error.HTTPError as err:
        raise RuntimeError(f"ingest into project 2: HTTP {err.code}") from err
    except OSError as err:
        raise Unreachable(f"{req.full_url}: {err}") from err


def produce_data():
    """Create issues in the sample app and wait until Bugsink has digested them; return the ids the sections use."""
    # Bugsink throttles ingestion at 1000 events per 5 minutes, so fill all variants only while issues are missing.
    existing = len(sum(all_pages(GLOBAL_READ, "issues/?project=1"), []))
    wanted = range(1, VARIANTS + 1) if existing < 2 * VARIANTS else {1, MUTE_VARIANT, VARIANTS}
    last = {}
    for variant in wanted:
        status, body = sample("GET", f"/handled?variant={variant}")
        if status == 200:
            last[variant] = body
        sample("GET", f"/error?variant={variant}")
    for _ in range(2):  # variant 1 gets three events in total, so event order can be read
        status, body = sample("GET", "/handled?variant=1")
        if status == 200:
            last[1] = body
            poll(lambda e=body["event_id"]: lookup_event(e), "extra variant-1 event")
    first, newest = last[1], last[VARIANTS]
    send_project2_event()
    status, no_trace = sample("POST", "/no-trace")
    if status != 200:
        raise RuntimeError(f"POST /no-trace: HTTP {status}")
    events = {name: poll(lambda e=ev["event_id"]: lookup_event(e), f"event {name}")
              for name, ev in (("first", first), ("newest", newest), ("no_trace", no_trace))}

    previous = []

    def settled():
        counts = [len(sum(all_pages(GLOBAL_READ, f"issues/?project={p}"), [])) for p in (1, 2)]
        stable = counts == previous and counts[1] > 0
        previous[:] = counts
        return counts if stable else None

    counts = poll(settled, "issue counts to settle")
    mute_issue = lookup_event(last[MUTE_VARIANT]["event_id"])["issue"]
    return {"trace_id": first["trace_id"], "event_id": first["event_id"], "issue": events["first"]["issue"],
            "newest": newest, "newest_event": events["newest"], "no_trace_event": events["no_trace"],
            "mute_issue": mute_issue, "issue_counts": counts, "variants": len(wanted)}


def fmt(path, ctx):
    return path.format(issue=ctx["issue"], event=ctx["event_id"])


def section_matrix(ctx):
    """Section 1: status of each endpoint for each token."""
    header = ["Request"] + list(TOKENS)
    rows = []
    for label, template in PROBES:
        cells = []
        for token in TOKENS.values():
            status, body = call(token, "GET", fmt(template, ctx))
            cell = str(status)
            if template == "projects/" and status == 200:
                cell += f" (lists project ids {sorted(p['id'] for p in body['results'])})"
            cells.append(cell)
        rows.append([label, *cells])
    cells = []
    for token in TOKENS.values():
        muted, mute_body = call(token, "POST", f"issues/{ctx['mute_issue']}/mute/")
        unmuted, _ = call(token, "POST", f"issues/{ctx['mute_issue']}/unmute/")
        if token == GLOBAL_TRIAGE:
            ctx["mute_body"] = mute_body
        cells.append(f"{muted} / {unmuted}")
    rows.append(["POST issues/<issue>/mute/ then unmute/", *cells])
    return table(header, rows) + (
        "\n\nCells are HTTP statuses; 401, 403 and 404 are answers. The last row shows the mute status, "
        "then the unmute status, on the `/handled?variant=60` issue.")


def section_refusals(ctx):
    """Section 2: the bodies of a 401 and of both forms of 403."""
    s401, b401 = call(None, "GET", "projects/")
    s403, b403 = call(NO_CAPS, "GET", "projects/")
    s403p, b403p = call(PROJECT_READ, "GET", "projects/2/")
    return "\n".join([
        f"- 401 (`GET projects/`, no token): status {s401}, body {code(b401)}",
        f"- 403 for a missing capability (`GET projects/`, NO_CAPS): status {s403}, body {code(b403)}; "
        f"names the capability `projects:read`: {'yes' if 'projects:read' in short(b403) else 'no'}",
        f"- 403 for another project (`GET projects/2/`, PROJECT_READ): status {s403p}, body {code(b403p)}; "
        f"names a capability: {'yes' if 'capability' in short(b403p) or ':read' in short(b403p) else 'no'}",
    ])


def distinct_projects(body):
    return sorted({i["project"] for i in body["results"]})


def section_no_project(ctx):
    """Section 3: GET issues/ without project (Q6)."""
    status, body = call(GLOBAL_READ, "GET", "issues/")
    if status != 200:
        return f"`GET issues/` with GLOBAL_READ: status {status}, body {code(body)}"
    status_p, _ = call(GLOBAL_READ, "GET", "issues/?project=1")
    return "\n".join([
        f"- `GET issues/` with GLOBAL_READ: status {status}",
        f"- Projects represented in the first page: {len(distinct_projects(body))} {distinct_projects(body)}",
        f"- Project 1 has {ctx['issue_counts'][0]} issues and project 2 has {ctx['issue_counts'][1]}; "
        f"the first page holds {len(body['results'])} results, `next` is {'set' if body['next'] else 'null'}",
        f"- `GET issues/?project=1` for comparison: status {status_p}",
    ])


def section_trace(ctx):
    """Section 4: finding an event or issue by trace id (Q7)."""
    trace = ctx["trace_id"]
    probes = [
        ("`GET events/?q=trace:<id>` (no issue)", f"events/?q=trace:{trace}"),
        ("`GET events/?issue=<issue>&q=trace:<id>`", f"events/?issue={ctx['issue']}&q=trace:{trace}"),
        ("`GET issues/?project=1&q=trace:<id>`", f"issues/?project=1&q=trace:{trace}"),
        ("`GET issues/?q=trace:<id>` (no project)", f"issues/?q=trace:{trace}"),
    ]
    rows, results = [], {}
    for label, path in probes:
        status, body = call(GLOBAL_READ, "GET", path)
        if status == 200 and isinstance(body, dict):
            issues = {r.get("issue") or r.get("id") for r in body["results"]}
            results[label] = len(body["results"])
            rows.append([label, status, len(body["results"]), "yes" if (ctx["issue"] in issues) else "no"])
        else:
            rows.append([label, status, "-", code(body)])
    no_issue_status = rows[0][1]
    verdict = ("Events cannot be searched by trace across issues: `events/` without `issue` answers "
               f"{no_issue_status}." if no_issue_status != 200 else
               f"`events/` without `issue` answered {no_issue_status} with {rows[0][2]} results.")
    return "\n".join([
        table(["Request", "Status", "Results", "Contains the trace's issue (or error)"], rows),
        "", verdict,
        f"\nExample line (varies per run): trace `{trace}`, issue `{ctx['issue']}`.",
    ])


def stamp_order(values):
    if len(values) < 2:
        return f"undetermined ({len(values)} values)"
    if all(a > b for a, b in zip(values, values[1:])):
        return "descending"
    if all(a < b for a, b in zip(values, values[1:])):
        return "ascending"
    return "mixed"


def section_paging(ctx):
    """Section 5: the shape of a page, the order of issues and the order of events (Q8)."""
    status, body = call(GLOBAL_READ, "GET", "issues/?sort=last_seen&order=desc")
    lines = [f"- Top-level keys: {sorted(body)}", f"- Page length: {len(body['results'])}",
             f"- `next` form: {'null' if body['next'] is None else re.sub(r'cursor=[^&]+', 'cursor=<opaque>', body['next'])}",
             f"- `previous` on the first page: {body['previous']}"]
    pages = all_pages(GLOBAL_READ, "issues/?sort=last_seen&order=desc")
    seen = [i["last_seen"] for page in pages for i in page]
    ordered = all(a >= b for a, b in zip(seen, seen[1:]))
    lines.append(f"- Following `next` to the end: {len(pages)} pages, {len(seen)} issues, page lengths "
                 f"{sorted({len(p) for p in pages})}; `last_seen` never increases: {'yes' if ordered else 'no'}")
    for label, path in (("`GET issues/` (all projects)", "issues/"), ("`GET issues/?project=1`", "issues/?project=1")):
        listed = [i for page in all_pages(GLOBAL_READ, path) for i in page]
        lines.append(f"- Default order of {label}: `last_seen` {stamp_order([i['last_seen'] for i in listed])}, "
                     f"`digest_order` {stamp_order([i['digest_order'] for i in listed])}")
    events = sum(all_pages(GLOBAL_READ, f"events/?issue={ctx['issue']}"), [])
    lines.append(f"- Default order of `GET events/?issue=<issue>` (at least two events): "
                 f"`digest_order` {stamp_order([e['digest_order'] for e in events])}, "
                 f"`timestamp` {stamp_order([e['timestamp'] for e in events])}, "
                 f"`ingested_at` {stamp_order([e['ingested_at'] for e in events])}")
    return "\n".join(lines)


def paths(value, prefix=""):
    """Flatten nested dicts into (dotted path, leaf) pairs."""
    if isinstance(value, dict):
        for key, inner in value.items():
            yield from paths(inner, f"{prefix}.{key}" if prefix else key)
    else:
        yield prefix, value


def section_event_data(ctx):
    """Section 6: where environment, release, trace and service live in event data (Q3)."""
    event, no_trace = ctx["newest_event"], ctx["no_trace_event"]
    data = event["data"]
    wanted = [(p, v) for p, v in paths(data)
              if p in ("environment", "release", "server_name") or p.startswith("contexts.trace.")
              or p == "tags.service" or p.startswith("tags.")]
    lines = ["Latest `/handled` event, selected JSON paths:", ""]
    ids = ("trace_id", "span_id")
    lines += [f"- `data.{p}` = {'`<varies per run>`' if p.endswith(ids) else code(v)}" for p, v in wanted]
    lines.append(f"- `data.tags` is a {type(data.get('tags')).__name__}")
    status, tags = call(GLOBAL_READ, "GET", f"issues/{event['issue']}/tags/")
    if status == 200:
        lines.append(f"- Tag keys from `issues/<issue>/tags/`: {sorted({t['key'] for t in tags['results']})}")
    else:
        lines.append(f"- `issues/<issue>/tags/`: status {status}")
    real, bare = data["contexts"]["trace"], no_trace["data"].get("contexts", {}).get("trace")
    lines += ["", f"`data.contexts.trace.trace_id` present on the `/no-trace` event: "
              f"{'yes' if isinstance(bare, dict) and 'trace_id' in bare else 'no'}"]
    if isinstance(bare, dict):
        lines.append(f"Keys only on the `/no-trace` event: {sorted(set(bare) - set(real))}; "
                     f"only on the `/handled` event: {sorted(set(real) - set(bare))}; "
                     f"values of the shared keys other than the ids are equal: "
                     f"{'yes' if all(real[k] == bare[k] for k in set(real) & set(bare) if k not in ids) else 'no'}")
    lines += ["", "Example lines (ids vary per run):",
              f"- `/handled` event `contexts.trace`: {code(real)}",
              f"- `/no-trace` event `contexts.trace`: {code(bare) if bare is not None else 'absent'}"]
    return "\n".join(lines)


def counts_summary(status, body):
    if status != 200 or not isinstance(body, dict) or "buckets" not in body:
        return f"status {status}, body {code(body)}"
    stamps = [b["timestamp"] for b in body["buckets"]]
    return (f"status {status}, keys {sorted(body)}, bucket keys {sorted(body['buckets'][0])}, "
            f"{len(stamps)} buckets, interval_seconds {body.get('interval_seconds')}")


def iso(moment):
    return moment.strftime("%Y-%m-%dT%H:%M:%SZ")


def section_event_counts(ctx):
    """Section 7: event-counts parameters and defaults (Q9)."""
    now = datetime.now(timezone.utc).replace(minute=0, second=0, microsecond=0)
    end = now + timedelta(hours=1)
    base = f"issues/{ctx['issue']}/event-counts/"
    cases = [
        ("`start`/`end` on hour boundaries, 24 h", f"?start={iso(end - timedelta(hours=24))}&end={iso(end)}"),
        ("`start` off the hour boundary", f"?start={iso(now - timedelta(hours=3) + timedelta(minutes=30))}"),
        ("`start` 91 days back", f"?start={iso(now - timedelta(days=91))}"),
        ("no parameters", ""),
        ("`end` only, on the hour", f"?end={iso(now)}"),
    ]
    lines, examples = [], []
    for label, query in cases:
        status, body = call(GLOBAL_READ, "GET", base + urllib.parse.quote(query, safe="?=&:"))
        lines.append(f"- {label}: {counts_summary(status, body)}")
        examples.append((label, body))
    default = examples[3][1]
    if isinstance(default, dict) and "buckets" in default:
        stamps = [b["timestamp"] for b in default["buckets"]]
        lines.append(f"- Defaults by observation: `end` is {'the end of the current hour' if default['end'] == iso(end) else 'not the end of the current hour'} "
                     f"(current hour starts at `{iso(now)}`), `start` is end minus "
                     f"{len(stamps)} h ({'24 h' if len(stamps) == 24 else 'not 24 h'}); buckets are "
                     f"{'in ascending order' if stamps == sorted(stamps) else 'not in ascending order'}")
        lines.append(f"\nExample line (varies per run): first bucket {code(default['buckets'][0])}, "
                     f"events counted in the window: {sum(b['count'] for b in default['buckets'])}")
    status, body = call(GLOBAL_READ, "GET", "projects/1/event-counts/")
    lines.append(f"- `projects/1/event-counts/` with no parameters: {counts_summary(status, body)}")
    return "\n".join(lines)


def section_triage(ctx):
    """Section 8: the body of a mute response (spec 4.E)."""
    body = ctx.get("mute_body")
    if not isinstance(body, dict):
        return f"`mute/` with GLOBAL_TRIAGE did not return an object: {code(body)}"
    status, issue = call(GLOBAL_READ, "GET", f"issues/{ctx['mute_issue']}/")
    same = isinstance(issue, dict) and sorted(issue) == sorted(body)
    return "\n".join([
        f"- Keys of the `mute/` response: {sorted(body)}",
        f"- Same keys as `GET issues/<issue>/`: {'yes' if same else 'no'}",
        f"- `is_muted` in the response: {body.get('is_muted')}",
    ])


def restore_issue(ctx):
    status, body = call(GLOBAL_READ, "GET", f"issues/{ctx['mute_issue']}/")
    if isinstance(body, dict) and body.get("is_muted"):
        call(GLOBAL_TRIAGE, "POST", f"issues/{ctx['mute_issue']}/unmute/")
        status, body = call(GLOBAL_READ, "GET", f"issues/{ctx['mute_issue']}/")
    return f"Check at the end of the run: the mute-test issue `is_muted` is {body.get('is_muted')}."


SECTIONS = [
    ("1. Matrix: which token may call what (Q5)", section_matrix),
    ("2. Refusals (Q4)", section_refusals),
    ("3. Issues without a project (Q6)", section_no_project),
    ("4. Events by trace (Q7)", section_trace),
    ("5. Paging and order (Q8)", section_paging),
    ("6. Event data (Q3)", section_event_data),
    ("7. Event counts (Q9)", section_event_counts),
    ("8. Triage response", section_triage),
]


def build_report():
    ctx = produce_data()
    out = ["# Bugsink API probe", "", f"Bugsink `{BUGSINK}`, sample app `{SAMPLE}`.", ""]
    out.append(f"Data: {ctx['variants']} `/handled` and {ctx['variants']} `/error` variants; "
               f"issues in project 1: {ctx['issue_counts'][0]}, in project 2: {ctx['issue_counts'][1]}.")
    for title, fn in SECTIONS:
        try:
            text = fn(ctx)
        except Unreachable:
            raise
        except Exception as err:  # noqa: BLE001 - one broken section must not stop the report
            text = f"SECTION FAILED: {type(err).__name__}: {err}"
        out += ["", f"## {title}", "", text]
    out += ["", "## Cleanup", "", restore_issue(ctx)]
    return "\n".join(out) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", help="write the report to this file instead of standard output")
    args = parser.parse_args()
    try:
        report = build_report()
    except Unreachable as err:
        print(f"unreachable: {err}")
        return 1
    except RuntimeError as err:
        print(f"cannot produce data: {err}")
        return 1
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            fh.write(report)
    else:
        sys.stdout.write(report)
    return 0


if __name__ == "__main__":
    sys.exit(main())
